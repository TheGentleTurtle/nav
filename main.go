package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const version = "1.0.1"

func init() {
	lipgloss.SetDefaultRenderer(lipgloss.NewRenderer(os.Stderr))
}

const shellWrapper = `# --- nav - terminal directory navigator ---
nav() {
  if [ $# -gt 0 ]; then
    command nav "$@"
    return
  fi
  local dir
  dir="$(NAV_WRAPPED=1 command nav)"
  if [ -n "$dir" ] && [ -d "$dir" ]; then
    cd "$dir"
  fi
}
# --- end nav ---`

const scrolloff = 3

const helpText = `nav v` + version + ` — terminal directory navigator
https://github.com/TheGentleTurtle/nav

Usage:
  nav                Open the file navigator
  nav --help         Show this help
  nav --version      Print version
  nav --setup        Run the shell wrapper setup
  nav --uninstall    Remove shell wrapper and uninstall nav

Navigation:
  hjkl / ↑↓←→         Navigate
  l / →                Enter folder
  h / ←                Go back
  enter                cd into selected folder and exit
  o                    Open in default app (Finder for folders)
  c                    Copy path to clipboard
  ~                    Jump to home directory
  [.]                  Toggle hidden files
  /                    Search (type to filter)
  q                    Quit

Search mode:
  type                 Filter files by name
  ↑↓                   Move cursor in results
  → / ←                Enter folder / go back
  /                    Accept results and browse them
  enter                cd into selection and exit
  esc                  Cancel search and clear filter
  backspace            Delete last character

More info: https://github.com/TheGentleTurtle/nav

Shell wrapper:
  nav requires a shell wrapper to change directories.
  Run 'nav --setup' or add this to your ~/.zshrc or ~/.bashrc:

` + shellWrapper + `
`

// --- Styles (yazi-inspired) ---
// Colors are plain values (no renderer needed)
var (
	dirColor  = lipgloss.Color("12")
	fileColor = lipgloss.Color("252")
	execColor = lipgloss.Color("10")
	imgColor  = lipgloss.Color("11")
	vidColor  = lipgloss.Color("13")
	archColor = lipgloss.Color("9")
	dotColor  = lipgloss.Color("242")
	cursorBg  = lipgloss.Color("237")
)

// Styles are initialized after init() sets the renderer
var (
	pathStyle      lipgloss.Style
	dimStyle       lipgloss.Style
	searchInput    lipgloss.Style
	matchStyle     lipgloss.Style
	posStyle       lipgloss.Style
	hiddenBadge    lipgloss.Style
	emptyStyle     lipgloss.Style
	flashStyle     lipgloss.Style
	pillMain       lipgloss.Style
	pillSep        lipgloss.Style
	searchPillMain lipgloss.Style
	searchPillSep  lipgloss.Style
	sizeStyle      lipgloss.Style
)

func initStyles() {
	pathStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
	dimStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("242"))
	searchInput = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)
	matchStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true).Underline(true)
	posStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Bold(true)
	hiddenBadge = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)
	emptyStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("242")).Italic(true)
	flashStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	pillMain = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(lipgloss.Color("12"))
	pillSep = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	searchPillMain = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(lipgloss.Color("11"))
	searchPillSep = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	sizeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("248"))
}

const (
	pillOpen  = "\ue0b6"
	pillClose = "\ue0b4"
)

// Nerd Font icons
var (
	iconFolder  = "\uf115"
	iconFile    = "\uf016"
	iconGo      = "\ue627"
	iconPython  = "\ue73c"
	iconJS      = "\ue74e"
	iconTS      = "\ue628"
	iconRust    = "\ue7a8"
	iconHTML    = "\ue736"
	iconCSS     = "\ue749"
	iconJSON    = "\ue60b"
	iconYAML    = "\ue6a8"
	iconMD      = "\ue73e"
	iconGit     = "\uf1d3"
	iconImage   = "\uf1c5"
	iconVideo   = "\uf1c8"
	iconAudio   = "\uf1c7"
	iconArchive = "\uf1c6"
	iconPDF     = "\uf1c1"
	iconConfig  = "\ue615"
	iconLock    = "\uf023"
	iconExec    = "\uf489"
	iconSwift   = "\ue755"
	iconRuby    = "\ue739"
	iconShell   = "\uf489"
	iconDocker  = "\ue7b0"
	iconText    = "\uf0f6"
)

func entryIcon(e os.DirEntry) string {
	if e.IsDir() {
		if strings.HasPrefix(e.Name(), ".") {
			return iconConfig
		}
		switch strings.ToLower(e.Name()) {
		case "node_modules", ".git", ".svn":
			return iconGit
		}
		return iconFolder
	}
	name := strings.ToLower(e.Name())
	switch name {
	case "dockerfile", "docker-compose.yml", "docker-compose.yaml":
		return iconDocker
	case "makefile", "cmakelists.txt":
		return iconConfig
	case ".gitignore", ".gitmodules", ".gitattributes":
		return iconGit
	case "license", "licence":
		return iconText
	case "go.mod", "go.sum":
		return iconGo
	case "package.json", "tsconfig.json":
		return iconJSON
	case "cargo.toml", "cargo.lock":
		return iconRust
	}
	ext := filepath.Ext(name)
	switch ext {
	case ".go":
		return iconGo
	case ".py", ".pyw", ".pyx":
		return iconPython
	case ".js", ".jsx", ".mjs", ".cjs":
		return iconJS
	case ".ts", ".tsx":
		return iconTS
	case ".rs":
		return iconRust
	case ".swift":
		return iconSwift
	case ".rb":
		return iconRuby
	case ".html", ".htm":
		return iconHTML
	case ".css", ".scss", ".sass", ".less":
		return iconCSS
	case ".json", ".jsonc":
		return iconJSON
	case ".yaml", ".yml":
		return iconYAML
	case ".md", ".mdx", ".markdown":
		return iconMD
	case ".sh", ".bash", ".zsh", ".fish":
		return iconShell
	case ".png", ".jpg", ".jpeg", ".gif", ".bmp", ".svg", ".webp", ".ico", ".tiff":
		return iconImage
	case ".mp4", ".mkv", ".avi", ".mov", ".webm", ".flv":
		return iconVideo
	case ".mp3", ".wav", ".flac", ".aac", ".ogg", ".m4a":
		return iconAudio
	case ".zip", ".tar", ".gz", ".bz2", ".xz", ".7z", ".rar", ".dmg":
		return iconArchive
	case ".pdf":
		return iconPDF
	case ".lock":
		return iconLock
	case ".toml", ".ini", ".cfg", ".conf", ".env":
		return iconConfig
	case ".txt", ".log", ".csv":
		return iconText
	}
	if info, err := e.Info(); err == nil {
		if info.Mode()&0111 != 0 {
			return iconExec
		}
	}
	return iconFile
}

// --- Messages ---

type clearFlashMsg struct{}

// --- Setup menu model ---

type setupChoice int

const (
	choiceAutomatic setupChoice = iota
	choiceManual
)

type setupModel struct {
	cursor setupChoice
	chosen setupChoice
	done   bool
}

func (m setupModel) Init() tea.Cmd { return nil }

func (m setupModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.done = true
			m.chosen = -1
			return m, tea.Quit
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			} else {
				m.cursor = choiceManual
			}
		case "j", "down":
			if m.cursor < choiceManual {
				m.cursor++
			} else {
				m.cursor = choiceAutomatic
			}
		case "enter":
			m.done = true
			m.chosen = m.cursor
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m setupModel) View() string {
	var b strings.Builder
	b.WriteString("  nav - terminal directory navigator\n\n")
	b.WriteString("  nav needs a shell wrapper to change directories.\n")
	b.WriteString("  This adds a small function to your shell config so that\n")
	b.WriteString("  pressing enter actually cd's into the selected folder.\n\n")
	options := []struct {
		label string
		desc  string
	}{
		{"Automatic", "Add the wrapper to your shell config automatically"},
		{"Manual", "Show the code to copy/paste yourself"},
	}
	for i, opt := range options {
		cursor := "  "
		if setupChoice(i) == m.cursor {
			cursor = "> "
		}
		b.WriteString(fmt.Sprintf("%s%-12s %s\n", cursor, opt.label, opt.desc))
	}
	b.WriteString("\n  hjkl/↑↓←→ | enter select | q quit\n")
	return b.String()
}

// --- File navigator model ---

type model struct {
	cwd        string
	entries    []os.DirEntry
	cursor     int
	offset     int
	height     int
	width      int
	err        string
	selected   string
	showHidden bool
	history    map[string]string
	filter     string
	filtering  bool
	allEntries []os.DirEntry
	flash      string
}

func sortEntries(entries []os.DirEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		di := entries[i].IsDir()
		dj := entries[j].IsDir()
		if di != dj {
			return di
		}
		return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
	})
}

func loadDir(path string, showHidden bool) ([]os.DirEntry, error) {
	all, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	entries := make([]os.DirEntry, 0, len(all))
	for _, e := range all {
		if !showHidden && strings.HasPrefix(e.Name(), ".") {
			continue
		}
		entries = append(entries, e)
	}
	sortEntries(entries)
	return entries, nil
}

func initialModel() model {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "/"
	}
	entries, _ := loadDir(cwd, false)
	return model{
		cwd:        cwd,
		entries:    entries,
		allEntries: entries,
		history:    make(map[string]string),
		height:     24,
		width:      80,
	}
}

func (m model) Init() tea.Cmd { return nil }

func (m *model) saveHistory() {
	if len(m.entries) > 0 && m.cursor < len(m.entries) {
		m.history[m.cwd] = m.entries[m.cursor].Name()
	}
}

func (m *model) restoreCursor() {
	if name, ok := m.history[m.cwd]; ok {
		for i, e := range m.entries {
			if e.Name() == name {
				m.cursor = i
				return
			}
		}
	}
	m.cursor = 0
}

func (m model) navigateTo(path string) model {
	entries, err := loadDir(path, m.showHidden)
	if err != nil {
		m.err = err.Error()
		return m
	}
	m.saveHistory()
	m.cwd = path
	m.entries = entries
	m.allEntries = entries
	m.filter = ""
	m.filtering = false
	m.err = ""
	m.restoreCursor()
	m.fixOffset()
	return m
}

func (m model) totalItems() int { return len(m.entries) }

func (m model) viewportHeight() int {
	reserved := 3
	if m.filtering || m.filter != "" {
		reserved++
	}
	h := m.height - reserved
	if h < 1 {
		h = 1
	}
	return h
}

func (m *model) fixOffset() {
	vh := m.viewportHeight()
	total := m.totalItems()
	if m.cursor >= total {
		m.cursor = total - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	if total <= vh {
		m.offset = 0
		return
	}
	if m.cursor-m.offset < scrolloff {
		m.offset = m.cursor - scrolloff
	}
	if m.cursor-m.offset >= vh-scrolloff {
		m.offset = m.cursor - vh + scrolloff + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
	if m.offset > total-vh {
		m.offset = total - vh
	}
}

func (m *model) applyFilter() {
	if m.filter == "" {
		m.entries = m.allEntries
	} else {
		lower := strings.ToLower(m.filter)
		filtered := make([]os.DirEntry, 0)
		for _, e := range m.allEntries {
			if strings.Contains(strings.ToLower(e.Name()), lower) {
				filtered = append(filtered, e)
			}
		}
		m.entries = filtered
	}
	m.cursor = 0
	m.offset = 0
}

// --- Update ---

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case clearFlashMsg:
		m.flash = ""
		return m, nil

	case tea.WindowSizeMsg:
		m.height = msg.Height
		m.width = msg.Width
		m.fixOffset()
		return m, nil

	case tea.KeyMsg:
		key := msg.String()

		if m.filtering {
			switch key {
			case "esc":
				m.filter = ""
				m.filtering = false
				m.entries = m.allEntries
				m.cursor = 0
				m.offset = 0
				m.fixOffset()
				return m, nil
			case "backspace":
				if len(m.filter) > 0 {
					m.filter = m.filter[:len(m.filter)-1]
					m.applyFilter()
					m.fixOffset()
				}
				return m, nil
			case "/":
				m.filtering = false
				return m, nil
			case "enter":
				if len(m.entries) > 0 {
					entry := m.entries[m.cursor]
					if entry.IsDir() {
						m.selected = filepath.Join(m.cwd, entry.Name())
					} else {
						m.selected = m.cwd
					}
				}
				return m, tea.Quit
			case "up":
				if m.cursor > 0 {
					m.cursor--
				} else {
					m.cursor = m.totalItems() - 1
				}
				m.fixOffset()
				return m, nil
			case "down":
				if m.cursor < m.totalItems()-1 {
					m.cursor++
				} else {
					m.cursor = 0
				}
				m.fixOffset()
				return m, nil
			case "right":
				if len(m.entries) > 0 && m.cursor < len(m.entries) {
					entry := m.entries[m.cursor]
					if entry.IsDir() {
						target := filepath.Join(m.cwd, entry.Name())
						m = m.navigateTo(target)
					}
				}
				return m, nil
			case "left":
				parent := filepath.Dir(m.cwd)
				if parent != m.cwd {
					m.history[parent] = filepath.Base(m.cwd)
					m = m.navigateTo(parent)
				}
				return m, nil
			default:
				if len(key) == 1 && key[0] >= 32 && key[0] <= 126 {
					m.filter += key
					m.applyFilter()
					m.fixOffset()
				}
				return m, nil
			}
		}

		switch key {
		case "q":
			return m, tea.Quit
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			} else {
				m.cursor = m.totalItems() - 1
			}
			m.fixOffset()
		case "j", "down":
			if m.cursor < m.totalItems()-1 {
				m.cursor++
			} else {
				m.cursor = 0
			}
			m.fixOffset()
		case "l", "right":
			if len(m.entries) > 0 {
				entry := m.entries[m.cursor]
				if entry.IsDir() {
					target := filepath.Join(m.cwd, entry.Name())
					m = m.navigateTo(target)
				}
			}
		case "h", "left":
			parent := filepath.Dir(m.cwd)
			if parent != m.cwd {
				m.history[parent] = filepath.Base(m.cwd)
				m = m.navigateTo(parent)
			}
		case "enter":
			if len(m.entries) > 0 {
				entry := m.entries[m.cursor]
				if entry.IsDir() {
					m.selected = filepath.Join(m.cwd, entry.Name())
				} else {
					m.selected = m.cwd
				}
			}
			return m, tea.Quit
		case ".":
			m.showHidden = !m.showHidden
			entries, err := loadDir(m.cwd, m.showHidden)
			if err == nil {
				var curName string
				if len(m.entries) > 0 && m.cursor < len(m.entries) {
					curName = m.entries[m.cursor].Name()
				}
				m.entries = entries
				m.allEntries = entries
				m.filter = ""
				m.cursor = 0
				for i, e := range m.entries {
					if e.Name() == curName {
						m.cursor = i
						break
					}
				}
				m.fixOffset()
			}
		case "o":
			if len(m.entries) > 0 && m.cursor < len(m.entries) {
				target := filepath.Join(m.cwd, m.entries[m.cursor].Name())
				exec.Command("open", target).Start()
			}
		case "c":
			if len(m.entries) > 0 && m.cursor < len(m.entries) {
				target := filepath.Join(m.cwd, m.entries[m.cursor].Name())
				cmd := exec.Command("pbcopy")
				cmd.Stdin = strings.NewReader(target)
				if cmd.Run() == nil {
					m.flash = "copied!"
					return m, tea.Tick(time.Second, func(t time.Time) tea.Msg {
						return clearFlashMsg{}
					})
				}
			}
		case "~":
			home, err := os.UserHomeDir()
			if err == nil {
				m = m.navigateTo(home)
			}
		case "/":
			m.filtering = true
			return m, nil
		}
	}
	return m, nil
}

// --- Helpers ---

func entryColor(e os.DirEntry) lipgloss.Color {
	if e.IsDir() {
		return dirColor
	}
	name := e.Name()
	if strings.HasPrefix(name, ".") {
		return dotColor
	}
	lname := strings.ToLower(name)
	if info, err := e.Info(); err == nil {
		if info.Mode()&0111 != 0 {
			return execColor
		}
	}
	for _, ext := range []string{".png", ".jpg", ".jpeg", ".gif", ".bmp", ".svg", ".webp", ".ico", ".tiff"} {
		if strings.HasSuffix(lname, ext) {
			return imgColor
		}
	}
	for _, ext := range []string{".mp4", ".mkv", ".avi", ".mov", ".mp3", ".wav", ".flac", ".aac", ".ogg"} {
		if strings.HasSuffix(lname, ext) {
			return vidColor
		}
	}
	for _, ext := range []string{".zip", ".tar", ".gz", ".bz2", ".xz", ".7z", ".rar", ".dmg"} {
		if strings.HasSuffix(lname, ext) {
			return archColor
		}
	}
	return fileColor
}

func humanSize(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	} else if bytes < 1024*1024 {
		return fmt.Sprintf("%.1f K", float64(bytes)/1024)
	} else if bytes < 1024*1024*1024 {
		return fmt.Sprintf("%.1f M", float64(bytes)/(1024*1024))
	}
	return fmt.Sprintf("%.1f G", float64(bytes)/(1024*1024*1024))
}

func countItems(path string) int {
	entries, err := os.ReadDir(path)
	if err != nil {
		return 0
	}
	return len(entries)
}

func highlightMatch(name string, filter string, baseColor lipgloss.Color) string {
	if filter == "" {
		return lipgloss.NewStyle().Foreground(baseColor).Render(name)
	}
	lowerName := strings.ToLower(name)
	lowerFilter := strings.ToLower(filter)
	idx := strings.Index(lowerName, lowerFilter)
	if idx == -1 {
		return lipgloss.NewStyle().Foreground(baseColor).Render(name)
	}
	before := name[:idx]
	match := name[idx : idx+len(filter)]
	after := name[idx+len(filter):]
	baseS := lipgloss.NewStyle().Foreground(baseColor)
	return baseS.Render(before) + matchStyle.Render(match) + baseS.Render(after)
}

func highlightMatchBg(name string, filter string, fg lipgloss.Color, bg lipgloss.Color) string {
	baseS := lipgloss.NewStyle().Foreground(fg).Background(bg)
	if filter == "" {
		return baseS.Render(name)
	}
	lowerName := strings.ToLower(name)
	lowerFilter := strings.ToLower(filter)
	idx := strings.Index(lowerName, lowerFilter)
	if idx == -1 {
		return baseS.Render(name)
	}
	before := name[:idx]
	match := name[idx : idx+len(filter)]
	after := name[idx+len(filter):]
	matchS := lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Background(bg).Bold(true).Underline(true)
	return baseS.Render(before) + matchS.Render(match) + baseS.Render(after)
}

// --- View ---

func (m model) View() string {
	var b strings.Builder

	pathDisplay := m.cwd
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(pathDisplay, home) {
		pathDisplay = "~" + pathDisplay[len(home):]
	}
	b.WriteString(pathStyle.Render("  "+pathDisplay) + "\n\n")

	vh := m.viewportHeight()
	total := m.totalItems()
	end := m.offset + vh
	if end > total {
		end = total
	}

	if total == 0 {
		b.WriteString(emptyStyle.Render("  (empty)") + "\n")
		for i := 1; i < vh; i++ {
			b.WriteString("\n")
		}
	} else {
		for i := m.offset; i < end; i++ {
			entry := m.entries[i]
			isCursor := m.cursor == i
			color := entryColor(entry)
			icon := entryIcon(entry)

			name := entry.Name()
			if entry.IsDir() {
				name += "/"
			}

			maxName := m.width - 8
			if maxName < 10 {
				maxName = 10
			}
			nameRunes := []rune(name)
			if len(nameRunes) > maxName {
				name = string(nameRunes[:maxName-1]) + "~"
			}

			if isCursor {
				openPill := lipgloss.NewStyle().Foreground(cursorBg).Render(pillOpen)
				closePill := lipgloss.NewStyle().Foreground(cursorBg).Render(pillClose)
				iconStr := lipgloss.NewStyle().Foreground(color).Background(cursorBg).Render(icon)
				spacer := lipgloss.NewStyle().Background(cursorBg).Render(" ")

				var styledName string
				if m.filter != "" {
					styledName = highlightMatchBg(name, m.filter, color, cursorBg)
				} else {
					styledName = lipgloss.NewStyle().Foreground(color).Background(cursorBg).Bold(true).Render(name)
				}

				contentWidth := lipgloss.Width(iconStr) + 1 + lipgloss.Width(styledName)
				padNeeded := m.width - contentWidth - 4
				if padNeeded < 0 {
					padNeeded = 0
				}
				padStr := lipgloss.NewStyle().Background(cursorBg).Render(strings.Repeat(" ", padNeeded))

				b.WriteString(" " + openPill + iconStr + spacer + styledName + padStr + closePill + "\n")
			} else {
				iconStr := lipgloss.NewStyle().Foreground(color).Render(icon)
				var coloredName string
				if m.filter != "" {
					coloredName = highlightMatch(name, m.filter, color)
				} else {
					coloredName = lipgloss.NewStyle().Foreground(color).Render(name)
				}
				b.WriteString("  " + iconStr + " " + coloredName + "\n")
			}
		}

		for i := end - m.offset; i < vh; i++ {
			b.WriteString("\n")
		}
	}

	if m.err != "" {
		b.WriteString("  Error: " + m.err + "\n")
	}

	if m.filtering || m.filter != "" {
		b.WriteString(searchInput.Render(fmt.Sprintf("  search: %s_", m.filter)) + "\n")
	}

	// --- Status bar ---
	var statusLeft string
	if m.filtering {
		statusLeft = searchPillSep.Render(pillOpen) +
			searchPillMain.Render(" SEARCH ") +
			searchPillSep.Render(pillClose) +
			dimStyle.Render("  ↑↓ move | → enter | ← back | / accept | enter cd | esc clear")
	} else {
		statusLeft = pillSep.Render(pillOpen) +
			pillMain.Render(" NAV ") +
			pillSep.Render(pillClose) +
			dimStyle.Render("  hjkl/↑↓←→ | o open | c copy | ~ home | [.] hidden | / search | q quit")
	}

	var rightParts []string

	if m.flash != "" {
		rightParts = append(rightParts, flashStyle.Render(m.flash))
	}

	if total > 0 && m.cursor < len(m.entries) {
		entry := m.entries[m.cursor]
		if entry.IsDir() {
			count := countItems(filepath.Join(m.cwd, entry.Name()))
			if count == 1 {
				rightParts = append(rightParts, sizeStyle.Render("1 item"))
			} else {
				rightParts = append(rightParts, sizeStyle.Render(fmt.Sprintf("%d items", count)))
			}
		} else {
			if info, err := entry.Info(); err == nil {
				rightParts = append(rightParts, sizeStyle.Render(humanSize(info.Size())))
			}
		}
	}

	if m.showHidden {
		rightParts = append(rightParts, hiddenBadge.Render("[H]"))
	}

	if total > 0 {
		rightParts = append(rightParts, posStyle.Render(fmt.Sprintf("%d/%d", m.cursor+1, total)))
	}

	statusRight := strings.Join(rightParts, "  ")
	leftLen := lipgloss.Width(statusLeft)
	rightLen := lipgloss.Width(statusRight)
	gap := m.width - leftLen - rightLen - 2
	if gap < 1 {
		gap = 1
	}

	b.WriteString("  " + statusLeft + strings.Repeat(" ", gap) + statusRight)

	return b.String()
}

// --- Setup & CLI ---

func detectShellRC() string {
	shell := os.Getenv("SHELL")
	home, _ := os.UserHomeDir()
	if strings.Contains(shell, "zsh") {
		return filepath.Join(home, ".zshrc")
	}
	return filepath.Join(home, ".bashrc")
}

func autoInstall() bool {
	rcFile := detectShellRC()
	content, err := os.ReadFile(rcFile)
	if err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "  Could not read %s: %v\n", rcFile, err)
		return false
	}
	if strings.Contains(string(content), "NAV_WRAPPED") {
		fmt.Fprintf(os.Stderr, "  Wrapper already installed in %s\n", rcFile)
		return true
	}
	f, err := os.OpenFile(rcFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  Could not write to %s: %v\n", rcFile, err)
		return false
	}
	defer f.Close()
	f.WriteString("\n\n" + shellWrapper + "\n")
	fmt.Fprintf(os.Stderr, "\n  Wrapper added to %s\n", rcFile)
	fmt.Fprintf(os.Stderr, "  Restart your terminal or run: source %s\n\n", rcFile)
	return true
}

func showManual() {
	fmt.Fprint(os.Stderr, "\n  Add this to your shell config:\n\n")
	fmt.Fprintln(os.Stderr, shellWrapper)
	fmt.Fprintf(os.Stderr, "\n  Restart your terminal or run: source %s\n\n", detectShellRC())
}

func removeWrapper() bool {
	rcFile := detectShellRC()
	content, err := os.ReadFile(rcFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  Could not read %s: %v\n", rcFile, err)
		return false
	}
	text := string(content)
	startMarker := "# --- nav - terminal directory navigator ---"
	endMarker := "# --- end nav ---"
	startIdx := strings.Index(text, startMarker)
	endIdx := strings.Index(text, endMarker)
	if startIdx == -1 || endIdx == -1 {
		fmt.Fprintln(os.Stderr, "  Shell wrapper not found in "+rcFile)
		return false
	}
	endIdx += len(endMarker)
	if endIdx < len(text) && text[endIdx] == '\n' {
		endIdx++
	}
	for startIdx > 0 && text[startIdx-1] == '\n' {
		startIdx--
	}
	newContent := text[:startIdx] + text[endIdx:]
	err = os.WriteFile(rcFile, []byte(newContent), 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  Could not write to %s: %v\n", rcFile, err)
		return false
	}
	fmt.Fprintf(os.Stderr, "  Removed shell wrapper from %s\n", rcFile)
	return true
}

func runSetup() {
	p := tea.NewProgram(setupModel{}, tea.WithOutput(os.Stderr))
	result, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	setup := result.(setupModel)
	if !setup.done || setup.chosen == -1 {
		os.Exit(0)
	}
	switch setup.chosen {
	case choiceAutomatic:
		autoInstall()
	case choiceManual:
		showManual()
	}
}

func main() {
	initStyles()

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--help", "-h":
			fmt.Fprint(os.Stderr, helpText)
			os.Exit(0)
		case "--version", "-v":
			fmt.Fprintf(os.Stderr, "nav v%s\n", version)
			os.Exit(0)
		case "--setup":
			runSetup()
			os.Exit(0)
		case "--uninstall":
			fmt.Fprintln(os.Stderr, "  Uninstalling nav...")
			removeWrapper()
			fmt.Fprintln(os.Stderr, "  Running: brew uninstall nav")
			cmd := exec.Command("brew", "uninstall", "nav")
			cmd.Stdout = os.Stderr
			cmd.Stderr = os.Stderr
			cmd.Run()
			fmt.Fprintln(os.Stderr, "\n  nav has been uninstalled. Restart your terminal to apply changes.")
			os.Exit(0)
		default:
			fmt.Fprintf(os.Stderr, "Unknown flag: %s\nRun 'nav --help' for usage.\n", os.Args[1])
			os.Exit(1)
		}
	}

	if os.Getenv("NAV_WRAPPED") != "1" {
		// If the wrapper is already in the shell config, remind the user to restart
		rcFile := detectShellRC()
		if content, err := os.ReadFile(rcFile); err == nil && strings.Contains(string(content), "NAV_WRAPPED") {
			fmt.Fprintf(os.Stderr, "  nav wrapper is installed. Restart your terminal or run: source %s\n", rcFile)
			os.Exit(0)
		}
		runSetup()
		os.Exit(0)
	}

	m := initialModel()
	p := tea.NewProgram(m, tea.WithOutput(os.Stderr), tea.WithAltScreen())

	result, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	final := result.(model)
	if final.selected != "" {
		fmt.Println(final.selected)
	}
}
