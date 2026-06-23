package main

import (
	"fmt"
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

func initialModel(root string, maxLevel int) model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = cursorStyle
	return model{root: root, maxLevel: maxLevel, loading: true, spin: s}
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

// version is injected at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	root := "."
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "-v", "--version", "version":
			fmt.Printf("pathsize %s\n", version)
			return
		case "-h", "--help", "help":
			fmt.Printf("pathsize %s\n\nUsage: pathsize [path] [depth]\n  path   directory to scan (default \".\")\n  depth  levels to expand, integer >= 1 (default 2)\n", version)
			return
		}
		root = os.Args[1]
	}
	maxLevel := 2
	if len(os.Args) > 2 {
		d, err := strconv.Atoi(os.Args[2])
		if err != nil || d < 1 {
			fmt.Fprintf(os.Stderr, "invalid depth: %s (must be integer >= 1)\n", os.Args[2])
			os.Exit(1)
		}
		maxLevel = d
	}
	abs, err := filepath.Abs(root)
	if err == nil {
		root = abs
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		fmt.Fprintf(os.Stderr, "not a directory: %s\n", root)
		os.Exit(1)
	}

	p := tea.NewProgram(initialModel(root, maxLevel), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
