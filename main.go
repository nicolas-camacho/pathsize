package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// node is a file or directory in the scanned tree.
type node struct {
	name     string
	path     string
	size     int64 // recursive total for dirs, file size for files
	isDir    bool
	level    int // 1 = first order, 2 = second order
	children []*node
	expanded bool
}

// dirSize returns the total size of a directory tree.
func dirSize(path string) int64 {
	var total int64
	filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if !d.IsDir() {
			if info, err := d.Info(); err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total
}

// scan builds the tree down to maxLevel (2). Sizes are always fully recursive.
func scan(root string, level, maxLevel int) ([]*node, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var nodes []*node
	for _, e := range entries {
		full := filepath.Join(root, e.Name())
		n := &node{name: e.Name(), path: full, isDir: e.IsDir(), level: level}
		if e.IsDir() {
			n.size = dirSize(full)
			if level < maxLevel {
				if kids, err := scan(full, level+1, maxLevel); err == nil {
					n.children = kids
				}
			}
		} else {
			if info, err := e.Info(); err == nil {
				n.size = info.Size()
			}
		}
		nodes = append(nodes, n)
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].size > nodes[j].size })
	return nodes, nil
}

// humanSize formats bytes as B/KB/MB/GB/TB.
func humanSize(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// parseSize parses a human-readable size like "10MB", "500K", "1.5G", "1024".
// Plain numbers are bytes. Returns 0 for an empty string.
func parseSize(s string) (int64, error) {
	s = strings.TrimSpace(strings.ToUpper(s))
	if s == "" {
		return 0, nil
	}
	i := 0
	for i < len(s) && (s[i] == '.' || (s[i] >= '0' && s[i] <= '9')) {
		i++
	}
	num := s[:i]
	unit := strings.TrimSpace(s[i:])
	val, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size: %s", s)
	}
	var mult float64 = 1
	switch unit {
	case "", "B":
		mult = 1
	case "K", "KB", "KIB":
		mult = 1 << 10
	case "M", "MB", "MIB":
		mult = 1 << 20
	case "G", "GB", "GIB":
		mult = 1 << 30
	case "T", "TB", "TIB":
		mult = 1 << 40
	default:
		return 0, fmt.Errorf("invalid size unit: %s", unit)
	}
	return int64(val * mult), nil
}

// --- styles ---
var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212")).MarginBottom(1)
	dirStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	fileStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
	sizeStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	cursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	selBg       = lipgloss.NewStyle().Background(lipgloss.Color("236"))
	helpStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).MarginTop(1)
)

type keymap struct {
	up, down, pageUp, pageDown, home, end, toggle, quit key.Binding
}

var keys = keymap{
	up:       key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	down:     key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	pageUp:   key.NewBinding(key.WithKeys("pgup", "ctrl+b"), key.WithHelp("pgup", "page up")),
	pageDown: key.NewBinding(key.WithKeys("pgdown", "ctrl+f"), key.WithHelp("pgdn", "page down")),
	home:     key.NewBinding(key.WithKeys("home", "g"), key.WithHelp("g", "top")),
	end:      key.NewBinding(key.WithKeys("end", "G"), key.WithHelp("G", "bottom")),
	toggle:   key.NewBinding(key.WithKeys("enter", " "), key.WithHelp("enter/space", "expand/collapse")),
	quit:     key.NewBinding(key.WithKeys("q", "ctrl+c", "esc"), key.WithHelp("q", "quit")),
}

type scanDoneMsg struct {
	nodes []*node
	err   error
}

type model struct {
	root     string
	maxLevel int     // deepest level scanned (>= 1)
	minSize  int64   // hide nodes smaller than this
	roots    []*node // top-level nodes
	cursor   int
	offset   int // index of first visible row (scroll position)
	height   int // terminal height
	width    int // terminal width
	loading  bool
	spin     spinner.Model
	err      error
	total    int64
}

func initialModel(root string, maxLevel int, minSize int64) model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = cursorStyle
	return model{root: root, maxLevel: maxLevel, minSize: minSize, loading: true, spin: s}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.spin.Tick, func() tea.Msg {
		nodes, err := scan(m.root, 1, m.maxLevel)
		return scanDoneMsg{nodes: nodes, err: err}
	})
}

// flatten returns currently visible nodes respecting expanded state.
func (m model) flatten() []*node {
	var out []*node
	var walk func(ns []*node)
	walk = func(ns []*node) {
		for _, n := range ns {
			if n.size < m.minSize {
				break // sorted largest-first: rest are smaller too
			}
			out = append(out, n)
			if n.isDir && n.expanded {
				walk(n.children)
			}
		}
	}
	walk(m.roots)
	return out
}

// listHeight is how many rows are available for the list (terminal minus chrome).
func (m model) listHeight() int {
	h := m.height - 4 // title(2) + help(1) + scroll hint(1)
	if h < 1 {
		h = 1
	}
	return h
}

// clampScroll keeps the cursor inside the visible window.
func (m *model) clampScroll() {
	lh := m.listHeight()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+lh {
		m.offset = m.cursor - lh + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height
		m.width = msg.Width
		m.clampScroll()
		return m, nil
	case scanDoneMsg:
		m.loading = false
		m.err = msg.err
		m.roots = msg.nodes
		for _, n := range m.roots {
			m.total += n.size
		}
		return m, nil
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.quit):
			return m, tea.Quit
		case key.Matches(msg, keys.up):
			if m.cursor > 0 {
				m.cursor--
			}
			m.clampScroll()
		case key.Matches(msg, keys.down):
			if m.cursor < len(m.flatten())-1 {
				m.cursor++
			}
			m.clampScroll()
		case key.Matches(msg, keys.pageUp):
			m.cursor -= m.listHeight()
			if m.cursor < 0 {
				m.cursor = 0
			}
			m.clampScroll()
		case key.Matches(msg, keys.pageDown):
			m.cursor += m.listHeight()
			if max := len(m.flatten()) - 1; m.cursor > max {
				m.cursor = max
			}
			m.clampScroll()
		case key.Matches(msg, keys.home):
			m.cursor = 0
			m.clampScroll()
		case key.Matches(msg, keys.end):
			m.cursor = len(m.flatten()) - 1
			m.clampScroll()
		case key.Matches(msg, keys.toggle):
			vis := m.flatten()
			if m.cursor < len(vis) {
				n := vis[m.cursor]
				if n.isDir && len(n.children) > 0 {
					n.expanded = !n.expanded
				}
			}
			m.clampScroll()
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m model) View() string {
	if m.loading {
		return fmt.Sprintf("\n %s Scanning %s ...\n", m.spin.View(), m.root)
	}
	if m.err != nil {
		return fmt.Sprintf("\n Error: %v\n", m.err)
	}

	vis := m.flatten()
	lh := m.listHeight()

	start := m.offset
	end := start + lh
	if end > len(vis) {
		end = len(vis)
	}

	b := titleStyle.Render(fmt.Sprintf("📁 %s  —  %s total  (%d items)", m.root, humanSize(m.total), len(vis))) + "\n"

	for i := start; i < end; i++ {
		n := vis[i]
		indent := ""
		if n.level > 1 {
			indent = strings.Repeat("   ", n.level-1)
		}
		cursor := "  "
		if i == m.cursor {
			cursor = cursorStyle.Render("▶ ")
		}
		var icon, name string
		if n.isDir {
			arrow := "▸"
			if n.expanded {
				arrow = "▾"
			}
			if len(n.children) == 0 {
				arrow = " "
			}
			icon = arrow + " 📁"
			name = dirStyle.Render(n.name)
		} else {
			icon = "  📄"
			name = fileStyle.Render(n.name)
		}
		line := fmt.Sprintf("%s%s%s %s  %s", cursor, indent, icon, name, sizeStyle.Render(humanSize(n.size)))
		if i == m.cursor {
			line = selBg.Render(line)
		}
		b += line + "\n"
	}

	// scroll hint: position + more above/below indicators
	scroll := ""
	if len(vis) > 0 {
		up, down := "  ", "  "
		if start > 0 {
			up = "↑ "
		}
		if end < len(vis) {
			down = "↓ "
		}
		scroll = helpStyle.Render(fmt.Sprintf("%s%s%d–%d / %d", up, down, start+1, end, len(vis)))
	}
	b += scroll + "\n"
	b += helpStyle.Render("↑/↓ nav · pgup/pgdn page · g/G top/bottom · enter expand · q quit")
	return b
}

// outNode is the JSON/serializable form of a node.
type outNode struct {
	Name     string    `json:"name"`
	Path     string    `json:"path"`
	Size     int64     `json:"size"`
	IsDir    bool      `json:"is_dir"`
	Children []outNode `json:"children,omitempty"`
}

// report is the top-level JSON document.
type report struct {
	Root  string    `json:"root"`
	Total int64     `json:"total"`
	Items []outNode `json:"items"`
}

// toOut converts a node tree to outNodes, dropping entries below minSize.
func toOut(nodes []*node, minSize int64) []outNode {
	var out []outNode
	for _, n := range nodes {
		if n.size < minSize {
			break // sorted largest-first
		}
		o := outNode{Name: n.name, Path: n.path, Size: n.size, IsDir: n.isDir}
		if n.isDir {
			o.Children = toOut(n.children, minSize)
		}
		out = append(out, o)
	}
	return out
}

// printPlain writes the tree as an indented plain-text listing.
func printPlain(w io.Writer, root string, total, minSize int64, nodes []*node) {
	fmt.Fprintf(w, "%s\t%s total\n", root, humanSize(total))
	var walk func(ns []*node, depth int)
	walk = func(ns []*node, depth int) {
		for _, n := range ns {
			if n.size < minSize {
				break
			}
			name := n.name
			if n.isDir {
				name += string(os.PathSeparator)
			}
			fmt.Fprintf(w, "%s%s\t%s\n", strings.Repeat("  ", depth), name, humanSize(n.size))
			if n.isDir {
				walk(n.children, depth+1)
			}
		}
	}
	walk(nodes, 1)
}

// version is injected at build time via -ldflags "-X main.version=...".
var version = "dev"

const usageText = `pathsize %s

Usage: pathsize [flags] [path] [depth]

  path    directory to scan (default ".")
  depth   levels to expand, integer >= 1 (default 2)

Flags:
  -v, --version      print version and exit
  -h, --help         print this help and exit
      --no-tui       print results as plain text instead of the TUI
      --json         print results as JSON (implies --no-tui)
      --min-size S   hide entries smaller than S (e.g. 10MB, 500K, 1.5G)

Note: flags must come before the positional path/depth arguments.
`

func main() {
	fs := flag.NewFlagSet("pathsize", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { fmt.Fprintf(os.Stderr, usageText, version) }

	showVer := fs.Bool("v", false, "print version and exit")
	showVer2 := fs.Bool("version", false, "print version and exit")
	noTUI := fs.Bool("no-tui", false, "print results as plain text instead of the TUI")
	jsonOut := fs.Bool("json", false, "print results as JSON (implies --no-tui)")
	minSizeStr := fs.String("min-size", "", "hide entries smaller than this (e.g. 10MB)")

	if err := fs.Parse(os.Args[1:]); err != nil {
		if err == flag.ErrHelp {
			return
		}
		os.Exit(2)
	}
	if *showVer || *showVer2 {
		fmt.Printf("pathsize %s\n", version)
		return
	}

	args := fs.Args()
	// Backward-compatible bare "version"/"help" words.
	if len(args) > 0 {
		switch args[0] {
		case "version":
			fmt.Printf("pathsize %s\n", version)
			return
		case "help":
			fs.Usage()
			return
		}
	}

	root := "."
	if len(args) > 0 {
		root = args[0]
	}
	maxLevel := 2
	if len(args) > 1 {
		d, err := strconv.Atoi(args[1])
		if err != nil || d < 1 {
			fmt.Fprintf(os.Stderr, "invalid depth: %s (must be integer >= 1)\n", args[1])
			os.Exit(1)
		}
		maxLevel = d
	}

	minSize, err := parseSize(*minSizeStr)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		fmt.Fprintf(os.Stderr, "not a directory: %s\n", root)
		os.Exit(1)
	}

	// Headless output modes: scan, print, exit (no TUI).
	if *jsonOut || *noTUI {
		nodes, err := scan(root, 1, maxLevel)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		var total int64
		for _, n := range nodes {
			total += n.size
		}
		if *jsonOut {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			if err := enc.Encode(report{Root: root, Total: total, Items: toOut(nodes, minSize)}); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
		} else {
			printPlain(os.Stdout, root, total, minSize, nodes)
		}
		return
	}

	p := tea.NewProgram(initialModel(root, maxLevel, minSize), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
