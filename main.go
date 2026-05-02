package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"
)

const version = "1.1.3"

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
  hjkl / arrows        Navigate (cursor wraps)
  l / right            Enter folder
  h / left             Go back
  ~                    Jump to home directory

Action:
  enter                cd (folder = into; file = current dir)
  space                cd into current directory (ignore cursor)
  o                    Open in default app
  R                    Reveal selected item in Finder
  t                    Show file tree (interactive)
  c                    Copy path to clipboard

View:
  /                    Fuzzy search
  s                    Cycle sort: name -> modified -> size
  [.]                  Toggle hidden files

System:
  ,                    Open settings
  ?                    Open help (also accessible from settings)
  q                    Quit

Tree mode:
  left/right           Decrease/increase depth (0 -> infinite -> 0)
  f                    Toggle files vs folders only
  .                    Toggle hidden files
  i                    Toggle skip ignored (node_modules, .git, etc.)
  m                    Toggle format: ASCII or Markdown
  c                    Copy tree to clipboard
  esc/q                Back to file list

Settings:
  Settings persist to ~/.config/nav/config.json
  Configurable: display (hidden, folders-on-top, sort), behavior (smart Enter),
    tree defaults (depth, files, hidden, skip-ignored, format)
  Smart Enter: when on, Enter on a file opens it instead of cd'ing

Search mode:
  type                 Fuzzy filter files by name
  up/down              Move cursor in results
  right / left         Enter folder / go back
  /                    Accept results and browse them
  enter                cd into folder (or open file with smart Enter)
  esc                  Clear filter
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

func (m model) pillOpenStr() string {
	if m.config.NerdFont {
		return pillOpen
	}
	return ""
}

func (m model) pillCloseStr() string {
	if m.config.NerdFont {
		return pillClose
	}
	return ""
}

func (m model) entryIconStr(e os.DirEntry) string {
	if !m.config.NerdFont {
		return ""
	}
	return entryIcon(e)
}

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

// --- Setup flow (single state-machine program) ---

type setupStep int

const (
	stepWrapper setupStep = iota
	stepShowManual
	stepNerdFont
)

type setupFlowModel struct {
	step          setupStep
	cursor        int
	wrapperPicked string // "automatic" or "manual" or ""
	nfPicked      string // "yes" or "no" or ""
	wrapperMsg    string // result of autoInstall
	cancelled     bool
	finalMsg      string
}

func (m setupFlowModel) Init() tea.Cmd { return nil }

func (m setupFlowModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	key := keyMsg.String()

	switch m.step {
	case stepWrapper:
		switch key {
		case "q", "ctrl+c":
			m.cancelled = true
			return m, tea.Quit
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			} else {
				m.cursor = 1
			}
		case "j", "down":
			if m.cursor < 1 {
				m.cursor++
			} else {
				m.cursor = 0
			}
		case "enter":
			if m.cursor == 0 {
				m.wrapperPicked = "automatic"
				m.wrapperMsg = autoInstall()
				m.step = stepNerdFont
				m.cursor = 0
			} else {
				m.wrapperPicked = "manual"
				m.step = stepShowManual
				m.cursor = 0
			}
		}
	case stepShowManual:
		switch key {
		case "q", "ctrl+c", "esc":
			m.cancelled = true
			return m, tea.Quit
		case "enter", " ":
			m.step = stepNerdFont
			m.cursor = 0
		}
	case stepNerdFont:
		switch key {
		case "q", "ctrl+c", "esc":
			m.cancelled = true
			return m, tea.Quit
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			} else {
				m.cursor = 1
			}
		case "j", "down":
			if m.cursor < 1 {
				m.cursor++
			} else {
				m.cursor = 0
			}
		case "y", "Y":
			m.nfPicked = "yes"
			return m, tea.Quit
		case "n", "N":
			m.nfPicked = "no"
			return m, tea.Quit
		case "enter":
			if m.cursor == 0 {
				m.nfPicked = "yes"
			} else {
				m.nfPicked = "no"
			}
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m setupFlowModel) View() string {
	switch m.step {
	case stepWrapper:
		return m.viewWrapper()
	case stepShowManual:
		return m.viewManual()
	case stepNerdFont:
		return m.viewNerdFont()
	}
	return ""
}

func (m setupFlowModel) viewWrapper() string {
	var b strings.Builder
	b.WriteString("\n  nav - terminal directory navigator\n\n")
	b.WriteString("  Step 1 of 2: Shell wrapper\n\n")
	b.WriteString("  nav needs a shell wrapper to change your working directory.\n")
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
		if i == m.cursor {
			cursor = "> "
		}
		b.WriteString(fmt.Sprintf("%s%-12s %s\n", cursor, opt.label, opt.desc))
	}
	b.WriteString("\n  hjkl/arrows | enter select | q quit\n")
	return b.String()
}

func (m setupFlowModel) viewManual() string {
	var b strings.Builder
	b.WriteString("\n  Manual wrapper setup\n\n")
	b.WriteString("  Add this to your ")
	b.WriteString(detectShellRC())
	b.WriteString(":\n\n")
	b.WriteString(shellWrapper)
	b.WriteString("\n\n")
	b.WriteString("  Then restart your terminal or run: source ")
	b.WriteString(detectShellRC())
	b.WriteString("\n\n")
	b.WriteString("  press enter to continue | esc to quit\n")
	return b.String()
}

func (m setupFlowModel) viewNerdFont() string {
	var b strings.Builder
	b.WriteString("\n  nav - terminal directory navigator\n\n")
	b.WriteString("  Step 2 of 2: Nerd Font\n\n")
	b.WriteString("  Does your terminal use a Nerd Font?\n\n")
	b.WriteString("  Nerd Fonts give nav file icons and a rounded cursor.\n")
	b.WriteString("  Without one, those characters render as ?? boxes.\n\n")
	b.WriteString("  If unsure: pick No. You'll get a clean plain-text UI.\n")
	b.WriteString("  Get one: brew install --cask font-jetbrains-mono-nerd-font\n\n")
	options := []struct {
		label string
		desc  string
	}{
		{"Yes", "I have a Nerd Font installed (icons + pretty UI)"},
		{"No / not sure", "Plain text mode (clean caret-style)"},
	}
	for i, opt := range options {
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}
		b.WriteString(fmt.Sprintf("%s%-16s %s\n", cursor, opt.label, opt.desc))
	}
	b.WriteString("\n  hjkl/arrows | y/n | enter select | esc skip\n")
	return b.String()
}

// --- Config ---

type Config struct {
	ShowHidden   bool   `json:"show_hidden"`
	FoldersOnTop bool   `json:"folders_on_top"`
	SortMode     string `json:"sort_mode"`
	SmartEnter   bool   `json:"smart_enter"`
	NerdFont     bool   `json:"nerd_font"`

	TreeDepth   int    `json:"tree_depth"` // -1 = ∞
	TreeFiles   bool   `json:"tree_files"`
	TreeHidden  bool   `json:"tree_hidden"`
	TreeIgnored bool   `json:"tree_skip_ignored"`
	TreeFormat  string `json:"tree_format"` // "ascii" or "md"
}

func detectNerdFont() bool {
	// Default ON. Setup asks the user during install if they have a Nerd Font;
	// their answer overrides this guess. Apple Terminal usually doesn't have one
	// configured, so default to OFF for that one.
	if os.Getenv("TERM_PROGRAM") == "Apple_Terminal" {
		return false
	}
	return true
}

func defaultConfig() Config {
	return Config{
		ShowHidden:   false,
		FoldersOnTop: true,
		SortMode:     "name",
		SmartEnter:   false,
		NerdFont:     detectNerdFont(),

		TreeDepth:   0,
		TreeFiles:   true,
		TreeHidden:  false,
		TreeIgnored: true,
		TreeFormat:  "ascii",
	}
}

func configPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "nav", "config.json")
}

func loadConfig() Config {
	cfg := defaultConfig()
	data, err := os.ReadFile(configPath())
	if err != nil {
		return cfg
	}
	json.Unmarshal(data, &cfg)
	return cfg
}

func saveConfig(cfg Config) error {
	path := configPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func parseSortMode(s string) sortMode {
	switch s {
	case "modified":
		return sortModified
	case "size":
		return sortSize
	}
	return sortName
}

func sortModeString(m sortMode) string {
	switch m {
	case sortModified:
		return "modified"
	case sortSize:
		return "size"
	}
	return "name"
}

// --- File navigator model ---

type viewMode int

const (
	modeNormal viewMode = iota
	modeSettings
	modeTree
	modeHelp
)

type treeOptions struct {
	root          string
	depth         int // -1 means unlimited
	includeFiles  bool
	includeHidden bool
	skipIgnored   bool
	format        string // "ascii" or "md"
}

func treeOptionsFromConfig(root string, cfg Config) treeOptions {
	format := cfg.TreeFormat
	if format != "ascii" && format != "md" {
		format = "ascii"
	}
	return treeOptions{
		root:          root,
		depth:         cfg.TreeDepth,
		includeFiles:  cfg.TreeFiles,
		includeHidden: cfg.TreeHidden,
		skipIgnored:   cfg.TreeIgnored,
		format:        format,
	}
}

type sortMode int

const (
	sortName sortMode = iota
	sortModified
	sortSize
)

func (s sortMode) label() string {
	switch s {
	case sortModified:
		return "mod"
	case sortSize:
		return "size"
	}
	return "name"
}

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

	mode         viewMode
	sortMode     sortMode
	foldersOnTop bool
	config       Config

	settingsCursor int
	settingsDraft  Config

	tree         treeOptions
	treeOutput   string
	treeCount    int
	treeTrunc    bool
	treeHitLimit bool
	treeMaxDepth int

	helpScroll int

	settingsConfirming    bool
	settingsConfirmAction string // "save" or "cancel"

	matchIndices map[string][]int
}

func sortEntries(entries []os.DirEntry, mode sortMode, foldersOnTop bool) {
	sort.SliceStable(entries, func(i, j int) bool {
		if foldersOnTop {
			di := entries[i].IsDir()
			dj := entries[j].IsDir()
			if di != dj {
				return di
			}
		}
		switch mode {
		case sortModified:
			ii, ei := entries[i].Info()
			ij, ej := entries[j].Info()
			if ei != nil || ej != nil {
				return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
			}
			return ii.ModTime().After(ij.ModTime())
		case sortSize:
			ii, ei := entries[i].Info()
			ij, ej := entries[j].Info()
			if ei != nil || ej != nil {
				return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
			}
			si := ii.Size()
			sj := ij.Size()
			if entries[i].IsDir() {
				si = 0
			}
			if entries[j].IsDir() {
				sj = 0
			}
			if si != sj {
				return si > sj
			}
			return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
		}
		return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
	})
}

func loadDir(path string, showHidden bool, mode sortMode, foldersOnTop bool) ([]os.DirEntry, error) {
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
	sortEntries(entries, mode, foldersOnTop)
	return entries, nil
}

func initialModel() model {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "/"
	}
	cfg := loadConfig()
	sortM := parseSortMode(cfg.SortMode)
	entries, _ := loadDir(cwd, cfg.ShowHidden, sortM, cfg.FoldersOnTop)
	return model{
		cwd:          cwd,
		entries:      entries,
		allEntries:   entries,
		history:      make(map[string]string),
		height:       24,
		width:        80,
		showHidden:   cfg.ShowHidden,
		sortMode:     sortM,
		foldersOnTop: cfg.FoldersOnTop,
		config:       cfg,
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
	entries, err := loadDir(path, m.showHidden, m.sortMode, m.foldersOnTop)
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

func (m *model) reload() {
	entries, err := loadDir(m.cwd, m.showHidden, m.sortMode, m.foldersOnTop)
	if err != nil {
		m.err = err.Error()
		return
	}
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
	m.cursor = 0
	m.offset = 0
	if m.filter == "" {
		m.entries = m.allEntries
		m.matchIndices = nil
		return
	}
	names := make([]string, len(m.allEntries))
	for i, e := range m.allEntries {
		names[i] = e.Name()
	}
	matches := fuzzy.Find(m.filter, names)
	filtered := make([]os.DirEntry, len(matches))
	m.matchIndices = make(map[string][]int, len(matches))
	for i, match := range matches {
		filtered[i] = m.allEntries[match.Index]
		m.matchIndices[m.allEntries[match.Index].Name()] = match.MatchedIndexes
	}
	m.entries = filtered
}

func openInDefaultApp(path string) error {
	return exec.Command("open", path).Start()
}

// truncatePath shortens a path with ellipsis in the middle when it exceeds maxWidth.
// Preserves leading prefix (~ or first segment) and as much trailing context as fits.
// Example: ~/Documents/Code/Tools/nav/main.go → ~/Documents/.../nav/main.go
func truncatePath(path string, maxWidth int) string {
	if maxWidth < 10 || len(path) <= maxWidth {
		return path
	}
	parts := strings.Split(path, "/")
	if len(parts) < 4 {
		return path
	}
	first := parts[0]
	if first == "" {
		first = "/" + parts[1]
		parts = parts[1:]
	}
	last := parts[len(parts)-1]
	// Try to keep as many trailing segments as fit
	for keepEnd := len(parts) - 1; keepEnd >= 1; keepEnd-- {
		tail := strings.Join(parts[len(parts)-keepEnd:], "/")
		candidate := first + "/.../" + tail
		if len(candidate) <= maxWidth {
			return candidate
		}
	}
	if len(first+"/.../"+last) <= maxWidth {
		return first + "/.../" + last
	}
	return ".../" + last
}

func revealInFinder(path string) error {
	return exec.Command("open", "-R", path).Start()
}

// --- Tree ---

const treeMaxItems = 500

var treeIgnored = map[string]bool{
	"node_modules": true,
	".git":         true,
	".DS_Store":    true,
	"__pycache__":  true,
	".idea":        true,
	".vscode":      true,
	"dist":         true,
	"build":        true,
	"target":       true,
	".next":        true,
	".cache":       true,
	"vendor":       true,
}

type treeBuilder struct {
	opts     treeOptions
	out      strings.Builder
	count    int
	trunc    bool
	hitLimit bool
	maxDepth int
}

func hasContent(path string, opts treeOptions) bool {
	entries, err := os.ReadDir(path)
	if err != nil {
		return false
	}
	for _, e := range entries {
		name := e.Name()
		if !opts.includeHidden && strings.HasPrefix(name, ".") {
			continue
		}
		if opts.skipIgnored && treeIgnored[name] {
			continue
		}
		if !opts.includeFiles && !e.IsDir() {
			continue
		}
		return true
	}
	return false
}

func (tb *treeBuilder) walk(path string, prefix string, depth int) {
	if tb.trunc {
		return
	}
	if tb.opts.depth >= 0 && depth > tb.opts.depth {
		// Check whether there's anything we WOULD have shown if depth were larger
		if hasContent(path, tb.opts) {
			tb.hitLimit = true
		}
		return
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		tb.out.WriteString(prefix + dimStyle.Render("[?]") + "\n")
		return
	}
	filtered := make([]os.DirEntry, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if !tb.opts.includeHidden && strings.HasPrefix(name, ".") {
			continue
		}
		if tb.opts.skipIgnored && treeIgnored[name] {
			continue
		}
		if !tb.opts.includeFiles && !e.IsDir() {
			continue
		}
		filtered = append(filtered, e)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		di := filtered[i].IsDir()
		dj := filtered[j].IsDir()
		if di != dj {
			return di
		}
		return strings.ToLower(filtered[i].Name()) < strings.ToLower(filtered[j].Name())
	})

	for i, e := range filtered {
		if tb.count >= treeMaxItems {
			tb.trunc = true
			return
		}
		tb.count++
		if depth+1 > tb.maxDepth {
			tb.maxDepth = depth + 1
		}
		isLast := i == len(filtered)-1
		name := e.Name()
		isSymlink := e.Type()&os.ModeSymlink != 0
		isDir := e.IsDir()
		if isDir {
			name += "/"
		}
		if isSymlink {
			name += " →"
		}

		switch tb.opts.format {
		case "md":
			indent := strings.Repeat("  ", depth+1)
			tb.out.WriteString(indent + "- " + name + "\n")
		default: // ascii
			var connector, nextPrefix string
			if isLast {
				connector = "└── "
				nextPrefix = prefix + "    "
			} else {
				connector = "├── "
				nextPrefix = prefix + "│   "
			}
			tb.out.WriteString(prefix + connector + name + "\n")
			if isDir && !isSymlink {
				tb.walk(filepath.Join(path, e.Name()), nextPrefix, depth+1)
				continue
			}
		}

		if tb.opts.format == "md" && isDir && !isSymlink {
			tb.walk(filepath.Join(path, e.Name()), "", depth+1)
		}
	}
}

type treeResult struct {
	output    string
	count     int
	truncated bool
	hitLimit  bool
	maxDepth  int
}

func buildTree(opts treeOptions) treeResult {
	tb := &treeBuilder{opts: opts}
	rootName := filepath.Base(opts.root) + "/"
	switch opts.format {
	case "md":
		tb.out.WriteString("- " + rootName + "\n")
	default:
		tb.out.WriteString(rootName + "\n")
	}
	tb.walk(opts.root, "", 0)
	if tb.trunc {
		tb.out.WriteString(fmt.Sprintf("\n... truncated at %d items\n", treeMaxItems))
	}
	return treeResult{
		output:    tb.out.String(),
		count:     tb.count,
		truncated: tb.trunc,
		hitLimit:  tb.hitLimit,
		maxDepth:  tb.maxDepth,
	}
}

func (m *model) rebuildTree() {
	r := buildTree(m.tree)
	m.treeOutput = r.output
	m.treeCount = r.count
	m.treeTrunc = r.truncated
	m.treeHitLimit = r.hitLimit
	m.treeMaxDepth = r.maxDepth
}

func (m model) updateTree(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc", "q":
		m.mode = modeNormal
		return m, nil
	case "left":
		// cycle: ∞ → max+ → ... → 1 → 0 → ∞
		if m.tree.depth == -1 {
			// from ∞ go to a sensible big number, then keep stepping down
			if m.treeMaxDepth > 0 {
				m.tree.depth = m.treeMaxDepth
			} else {
				m.tree.depth = 5
			}
		} else if m.tree.depth == 0 {
			m.tree.depth = -1
		} else {
			m.tree.depth--
		}
		m.rebuildTree()
		return m, nil
	case "right":
		// cycle: 0 → 1 → 2 → ... → ∞ → 0
		if m.tree.depth == -1 {
			m.tree.depth = 0
		} else {
			m.tree.depth++
		}
		m.rebuildTree()
		return m, nil
	case "f":
		m.tree.includeFiles = !m.tree.includeFiles
		m.rebuildTree()
		return m, nil
	case ".":
		m.tree.includeHidden = !m.tree.includeHidden
		m.rebuildTree()
		return m, nil
	case "i":
		m.tree.skipIgnored = !m.tree.skipIgnored
		m.rebuildTree()
		return m, nil
	case "m":
		if m.tree.format == "ascii" {
			m.tree.format = "md"
		} else {
			m.tree.format = "ascii"
		}
		m.rebuildTree()
		return m, nil
	case "c":
		cmd := exec.Command("pbcopy")
		cmd.Stdin = strings.NewReader(m.treeOutput)
		if cmd.Run() == nil {
			m.flash = "tree copied"
			return m, tea.Tick(time.Second, func(t time.Time) tea.Msg { return clearFlashMsg{} })
		}
		return m, nil
	}
	return m, nil
}

func (m model) renderTree() string {
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)
	chipStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("11"))

	rootDisplay := m.tree.root
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(rootDisplay, home) {
		rootDisplay = "~" + rootDisplay[len(home):]
	}

	depthStr := fmt.Sprintf("%d", m.tree.depth)
	if m.tree.depth == -1 {
		depthStr = "∞"
	}
	// Add (max) indicator if current depth shows everything (no further depth would add anything)
	if m.tree.depth != -1 && !m.treeHitLimit && !m.treeTrunc && m.tree.depth >= m.treeMaxDepth && m.treeMaxDepth > 0 {
		depthStr += " (max)"
	} else if m.tree.depth == -1 && !m.treeTrunc {
		depthStr += " (" + fmt.Sprintf("%d", m.treeMaxDepth) + " deep)"
	}
	filesStr := "off"
	if m.tree.includeFiles {
		filesStr = "on"
	}
	hiddenStr := "off"
	if m.tree.includeHidden {
		hiddenStr = "on"
	}
	ignoredStr := "skip"
	if !m.tree.skipIgnored {
		ignoredStr = "show"
	}

	b.WriteString("\n  " + titleStyle.Render("Tree: "+rootDisplay) + "\n")
	b.WriteString("  " + chipStyle.Render(fmt.Sprintf("depth: %s   files: %s   hidden: %s   format: %s   ignored: %s",
		depthStr, filesStr, hiddenStr, m.tree.format, ignoredStr)) + "\n\n")

	vh := m.height - 6
	if vh < 1 {
		vh = 1
	}

	lines := strings.Split(m.treeOutput, "\n")
	if len(lines) > vh {
		lines = lines[:vh]
		lines = append(lines, dimStyle.Render(fmt.Sprintf("  ... %d more lines (copy to see all)", len(strings.Split(m.treeOutput, "\n"))-vh)))
	}
	for _, line := range lines {
		b.WriteString("  " + line + "\n")
	}

	for i := len(lines); i < vh; i++ {
		b.WriteString("\n")
	}

	hint := dimStyle.Render(fmt.Sprintf("  %d items  |  ←→ depth | f files | . hidden | i ignored | m format | c copy | esc back",
		m.treeCount))
	if m.flash != "" {
		hint = dimStyle.Render(fmt.Sprintf("  %d items  |  ", m.treeCount)) + flashStyle.Render(m.flash)
	}
	b.WriteString(hint)
	return b.String()
}

// --- Help ---

type helpEntry struct {
	key  string
	desc string
}

type helpSection struct {
	title   string
	entries []helpEntry
}

func helpSections() []helpSection {
	return []helpSection{
		{"Navigation", []helpEntry{
			{"hjkl / arrows", "Move (l or right enters folder, h or left goes back)"},
			{"~", "Jump to home directory"},
		}},
		{"Action", []helpEntry{
			{"enter", "cd — folder = into; file = current dir"},
			{"space", "cd into current directory (ignore cursor)"},
			{"o", "Open in default app"},
			{"R", "Reveal in Finder"},
			{"t", "Show file tree (interactive view)"},
			{"c", "Copy path to clipboard"},
		}},
		{"View", []helpEntry{
			{"/", "Fuzzy search"},
			{"s", "Cycle sort: name -> modified -> size"},
			{".", "Toggle hidden files"},
		}},
		{"System", []helpEntry{
			{",", "Open settings"},
			{"?", "Open this help"},
			{"q", "Quit"},
		}},
		{"Tree mode", []helpEntry{
			{"←→", "Decrease/increase depth (0 → ∞ → 0)"},
			{"f", "Toggle files vs folders only"},
			{".", "Toggle hidden files"},
			{"i", "Toggle skip ignored (node_modules, .git, ...)"},
			{"m", "Toggle format: ASCII / Markdown"},
			{"c", "Copy tree to clipboard"},
			{"esc / q", "Back to file list"},
		}},
		{"Search mode", []helpEntry{
			{"type", "Fuzzy filter files by name"},
			{"↑↓", "Move cursor in results"},
			{"→ / ←", "Enter folder / go back"},
			{"/", "Accept results, browse them"},
			{"enter", "cd into selection"},
			{"esc", "Clear filter"},
		}},
		{"Settings mode", []helpEntry{
			{"j/k or ↑↓", "Move cursor"},
			{"space / enter", "Toggle bool / cycle enum"},
			{"←→", "Cycle enum value"},
			{"S", "Save and exit"},
			{"esc", "Cancel without saving"},
		}},
	}
}

func renderedHelpLines() []string {
	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)
	sectionStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)
	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Bold(true)
	descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	linkStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Underline(true)
	subStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("242"))

	var lines []string
	lines = append(lines, "")
	lines = append(lines, "  "+titleStyle.Render("nav v"+version+" — terminal directory navigator"))
	lines = append(lines, "  "+linkStyle.Render("https://github.com/TheGentleTurtle/nav"))
	lines = append(lines, "  "+subStyle.Render("Issues & feature requests: ")+linkStyle.Render("https://github.com/TheGentleTurtle/nav/issues"))
	lines = append(lines, "")
	for _, sec := range helpSections() {
		lines = append(lines, "  "+sectionStyle.Render(sec.title))
		for _, e := range sec.entries {
			keyW := 14
			k := e.key
			if len(k) > keyW {
				k = k[:keyW]
			}
			k = k + strings.Repeat(" ", keyW-len(k))
			lines = append(lines, "    "+keyStyle.Render(k)+"  "+descStyle.Render(e.desc))
		}
		lines = append(lines, "")
	}
	lines = append(lines, "  "+subStyle.Render("Settings file: ~/.config/nav/config.json"))
	lines = append(lines, "  "+subStyle.Render("License: CC BY-NC 4.0"))
	lines = append(lines, "")
	return lines
}

func (m model) updateHelp(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc", "q", "?":
		m.mode = modeNormal
		return m, nil
	case "k", "up":
		if m.helpScroll > 0 {
			m.helpScroll--
		}
	case "j", "down":
		lines := renderedHelpLines()
		vh := m.height - 2
		if m.helpScroll < len(lines)-vh {
			m.helpScroll++
		}
	case "g":
		m.helpScroll = 0
	case "G":
		lines := renderedHelpLines()
		vh := m.height - 2
		max := len(lines) - vh
		if max < 0 {
			max = 0
		}
		m.helpScroll = max
	}
	return m, nil
}

func (m model) renderHelp() string {
	var b strings.Builder
	lines := renderedHelpLines()
	vh := m.height - 2
	if vh < 1 {
		vh = 1
	}
	end := m.helpScroll + vh
	if end > len(lines) {
		end = len(lines)
	}
	for i := m.helpScroll; i < end; i++ {
		b.WriteString(lines[i] + "\n")
	}
	for i := end - m.helpScroll; i < vh; i++ {
		b.WriteString("\n")
	}
	hint := "  ↑↓/jk scroll | g top | G bottom | esc/q/? back"
	if len(lines) > vh {
		hint += fmt.Sprintf("   %d/%d", m.helpScroll+1, len(lines)-vh+1)
	}
	b.WriteString(dimStyle.Render(hint))
	return b.String()
}

// --- Settings ---

type settingField struct {
	label   string
	section string
	kind    string // "bool", "sort", "treeDepth", "treeFormat", "action"
}

func settingsFields() []settingField {
	return []settingField{
		{"Show hidden files (default)", "Display", "bool"},
		{"Folders always on top", "Display", "bool"},
		{"Sort mode (default)", "Display", "sort"},
		{"Nerd Font icons", "Display", "bool"},
		{"Smart Enter (open files instead of cd)", "Behavior", "bool"},
		{"Default depth", "Tree defaults", "treeDepth"},
		{"Include files (default)", "Tree defaults", "bool"},
		{"Include hidden files (default)", "Tree defaults", "bool"},
		{"Skip ignored (default)", "Tree defaults", "bool"},
		{"Format (default)", "Tree defaults", "treeFormat"},
		{"Open help", "Other", "action"},
		{"Open config file in editor", "Other", "action"},
		{"Reset all settings to defaults", "Other", "action"},
	}
}

func (m model) settingsChanged() bool {
	return m.settingsDraft != m.config
}

func (m model) updateSettings(key string) (tea.Model, tea.Cmd) {
	fields := settingsFields()

	// Confirm dialog for save/cancel
	if m.settingsConfirming {
		switch key {
		case "y", "Y", "enter":
			if m.settingsConfirmAction == "save" {
				m.config = m.settingsDraft
				m.showHidden = m.config.ShowHidden
				m.foldersOnTop = m.config.FoldersOnTop
				m.sortMode = parseSortMode(m.config.SortMode)
				saveConfig(m.config)
				m.reload()
				m.flash = "settings saved"
			} else {
				m.flash = "changes discarded"
			}
			m.settingsConfirming = false
			m.settingsConfirmAction = ""
			m.mode = modeNormal
			return m, tea.Tick(time.Second, func(t time.Time) tea.Msg { return clearFlashMsg{} })
		case "n", "N", "esc":
			m.settingsConfirming = false
			m.settingsConfirmAction = ""
			return m, nil
		}
		return m, nil
	}

	switch key {
	case "esc":
		if m.settingsChanged() {
			m.settingsConfirming = true
			m.settingsConfirmAction = "cancel"
			return m, nil
		}
		m.mode = modeNormal
		return m, nil
	case "q":
		if m.settingsChanged() {
			m.settingsConfirming = true
			m.settingsConfirmAction = "cancel"
			return m, nil
		}
		m.mode = modeNormal
		return m, nil
	case ",":
		if m.settingsChanged() {
			m.settingsConfirming = true
			m.settingsConfirmAction = "cancel"
			return m, nil
		}
		m.mode = modeNormal
		return m, nil
	case "k", "up":
		if m.settingsCursor > 0 {
			m.settingsCursor--
		} else {
			m.settingsCursor = len(fields) - 1
		}
		return m, nil
	case "j", "down":
		if m.settingsCursor < len(fields)-1 {
			m.settingsCursor++
		} else {
			m.settingsCursor = 0
		}
		return m, nil
	case " ", "enter":
		f := fields[m.settingsCursor]
		switch f.kind {
		case "bool":
			switch m.settingsCursor {
			case 0:
				m.settingsDraft.ShowHidden = !m.settingsDraft.ShowHidden
			case 1:
				m.settingsDraft.FoldersOnTop = !m.settingsDraft.FoldersOnTop
			case 3:
				m.settingsDraft.NerdFont = !m.settingsDraft.NerdFont
			case 4:
				m.settingsDraft.SmartEnter = !m.settingsDraft.SmartEnter
			case 6:
				m.settingsDraft.TreeFiles = !m.settingsDraft.TreeFiles
			case 7:
				m.settingsDraft.TreeHidden = !m.settingsDraft.TreeHidden
			case 8:
				m.settingsDraft.TreeIgnored = !m.settingsDraft.TreeIgnored
			}
		case "sort":
			cur := parseSortMode(m.settingsDraft.SortMode)
			cur = (cur + 1) % 3
			m.settingsDraft.SortMode = sortModeString(cur)
		case "treeDepth":
			// 0 → 1 → 2 → ... → 10 → ∞ → 0
			if m.settingsDraft.TreeDepth == -1 {
				m.settingsDraft.TreeDepth = 0
			} else if m.settingsDraft.TreeDepth >= 10 {
				m.settingsDraft.TreeDepth = -1
			} else {
				m.settingsDraft.TreeDepth++
			}
		case "treeFormat":
			if m.settingsDraft.TreeFormat == "ascii" {
				m.settingsDraft.TreeFormat = "md"
			} else {
				m.settingsDraft.TreeFormat = "ascii"
			}
		case "action":
			switch f.label {
			case "Open help":
				m.mode = modeHelp
				m.helpScroll = 0
			case "Open config file in editor":
				openInDefaultApp(configPath())
				m.flash = "opening config..."
				return m, tea.Tick(time.Second, func(t time.Time) tea.Msg { return clearFlashMsg{} })
			case "Reset all settings to defaults":
				saved := m.settingsDraft.NerdFont // preserve onboarding choice
				m.settingsDraft = defaultConfig()
				m.settingsDraft.NerdFont = saved
				m.flash = "settings reset (press S to save)"
				return m, tea.Tick(2*time.Second, func(t time.Time) tea.Msg { return clearFlashMsg{} })
			}
		}
		return m, nil
	case "left":
		f := fields[m.settingsCursor]
		switch f.kind {
		case "sort":
			cur := parseSortMode(m.settingsDraft.SortMode)
			cur = (cur + 2) % 3
			m.settingsDraft.SortMode = sortModeString(cur)
		case "treeDepth":
			if m.settingsDraft.TreeDepth == 0 {
				m.settingsDraft.TreeDepth = -1
			} else if m.settingsDraft.TreeDepth == -1 {
				m.settingsDraft.TreeDepth = 10
			} else {
				m.settingsDraft.TreeDepth--
			}
		case "treeFormat":
			if m.settingsDraft.TreeFormat == "ascii" {
				m.settingsDraft.TreeFormat = "md"
			} else {
				m.settingsDraft.TreeFormat = "ascii"
			}
		}
		return m, nil
	case "right":
		f := fields[m.settingsCursor]
		switch f.kind {
		case "sort":
			cur := parseSortMode(m.settingsDraft.SortMode)
			cur = (cur + 1) % 3
			m.settingsDraft.SortMode = sortModeString(cur)
		case "treeDepth":
			if m.settingsDraft.TreeDepth == -1 {
				m.settingsDraft.TreeDepth = 0
			} else {
				m.settingsDraft.TreeDepth++
			}
		case "treeFormat":
			if m.settingsDraft.TreeFormat == "ascii" {
				m.settingsDraft.TreeFormat = "md"
			} else {
				m.settingsDraft.TreeFormat = "ascii"
			}
		}
		return m, nil
	case "S", "ctrl+s":
		if !m.settingsChanged() {
			m.mode = modeNormal
			return m, nil
		}
		m.settingsConfirming = true
		m.settingsConfirmAction = "save"
		return m, nil
	}
	return m, nil
}

func (m model) renderSettings() string {
	var b strings.Builder
	fields := settingsFields()

	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)
	sectionStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	cursorIndicator := lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Render("▌")
	checkOn := lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render("[✓]")
	checkOff := lipgloss.NewStyle().Foreground(lipgloss.Color("242")).Render("[ ]")
	enumStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("11"))

	cfgPath := configPath()
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(cfgPath, home) {
		cfgPath = "~" + cfgPath[len(home):]
	}
	b.WriteString("\n  " + dimStyle.Render(cfgPath) + "\n")
	b.WriteString("  " + titleStyle.Render("Settings") + "\n\n")

	currentSection := ""
	for i, f := range fields {
		if f.section != currentSection {
			if currentSection != "" {
				b.WriteString("\n")
			}
			b.WriteString("  " + sectionStyle.Render(f.section) + "\n")
			currentSection = f.section
		}

		var value string
		switch f.kind {
		case "bool":
			var on bool
			switch i {
			case 0:
				on = m.settingsDraft.ShowHidden
			case 1:
				on = m.settingsDraft.FoldersOnTop
			case 3:
				on = m.settingsDraft.NerdFont
			case 4:
				on = m.settingsDraft.SmartEnter
			case 6:
				on = m.settingsDraft.TreeFiles
			case 7:
				on = m.settingsDraft.TreeHidden
			case 8:
				on = m.settingsDraft.TreeIgnored
			}
			if on {
				value = checkOn
			} else {
				value = checkOff
			}
		case "sort":
			value = enumStyle.Render("[ ‹ " + m.settingsDraft.SortMode + " › ]")
		case "treeDepth":
			d := fmt.Sprintf("%d", m.settingsDraft.TreeDepth)
			if m.settingsDraft.TreeDepth == -1 {
				d = "∞"
			}
			value = enumStyle.Render("[ ‹ " + d + " › ]")
		case "treeFormat":
			value = enumStyle.Render("[ ‹ " + m.settingsDraft.TreeFormat + " › ]")
		case "action":
			value = enumStyle.Render("[ enter ]")
		}

		indicator := "  "
		if i == m.settingsCursor {
			indicator = cursorIndicator + " "
		}
		labelW := 42
		label := f.label
		if len(label) < labelW {
			label = label + strings.Repeat(" ", labelW-len(label))
		}
		b.WriteString("    " + indicator + labelStyle.Render(label) + value + "\n")
	}

	for i := 0; i < m.height-len(fields)-9; i++ {
		b.WriteString("\n")
	}

	if m.settingsConfirming {
		var prompt string
		if m.settingsConfirmAction == "save" {
			prompt = "Save changes? (y/n)"
		} else {
			prompt = "Discard changes? (y/n)"
		}
		promptStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)
		b.WriteString("  " + promptStyle.Render(prompt))
		return b.String()
	}

	hint := dimStyle.Render("  ↑↓ move | space toggle | ←→ cycle | S save | esc cancel")
	b.WriteString(hint)
	return b.String()
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

		if m.mode == modeSettings {
			return m.updateSettings(key)
		}
		if m.mode == modeTree {
			return m.updateTree(key)
		}
		if m.mode == modeHelp {
			return m.updateHelp(key)
		}

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
					target := filepath.Join(m.cwd, entry.Name())
					if entry.IsDir() {
						m.selected = target
					} else if m.config.SmartEnter {
						openInDefaultApp(target)
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
				target := filepath.Join(m.cwd, entry.Name())
				if entry.IsDir() {
					m.selected = target
				} else if m.config.SmartEnter {
					openInDefaultApp(target)
				} else {
					m.selected = m.cwd
				}
			}
			return m, tea.Quit
		case " ":
			m.selected = m.cwd
			return m, tea.Quit
		case "R":
			target := m.cwd
			if len(m.entries) > 0 && m.cursor < len(m.entries) {
				target = filepath.Join(m.cwd, m.entries[m.cursor].Name())
			}
			revealInFinder(target)
			return m, nil
		case ",":
			m.mode = modeSettings
			m.settingsCursor = 0
			m.settingsDraft = m.config
			return m, nil
		case "t":
			root := m.cwd
			if len(m.entries) > 0 && m.cursor < len(m.entries) {
				entry := m.entries[m.cursor]
				if entry.IsDir() {
					root = filepath.Join(m.cwd, entry.Name())
				}
			}
			m.tree = treeOptionsFromConfig(root, m.config)
			m.rebuildTree()
			m.mode = modeTree
			return m, nil
		case "?":
			m.mode = modeHelp
			m.helpScroll = 0
			return m, nil
		case "home":
			home, err := os.UserHomeDir()
			if err == nil {
				m = m.navigateTo(home)
			}
			return m, nil
		case "s":
			m.sortMode = (m.sortMode + 1) % 3
			m.reload()
			m.flash = "sort: " + m.sortMode.label()
			return m, tea.Tick(time.Second, func(t time.Time) tea.Msg { return clearFlashMsg{} })
		case ".":
			m.showHidden = !m.showHidden
			m.reload()
		case "o":
			target := m.cwd
			if len(m.entries) > 0 && m.cursor < len(m.entries) {
				target = filepath.Join(m.cwd, m.entries[m.cursor].Name())
			}
			exec.Command("open", target).Start()
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

func highlightFuzzy(name string, indices []int, baseColor lipgloss.Color) string {
	baseS := lipgloss.NewStyle().Foreground(baseColor)
	if len(indices) == 0 {
		return baseS.Render(name)
	}
	hit := make(map[int]bool, len(indices))
	for _, i := range indices {
		hit[i] = true
	}
	var b strings.Builder
	for i, r := range name {
		if hit[i] {
			b.WriteString(matchStyle.Render(string(r)))
		} else {
			b.WriteString(baseS.Render(string(r)))
		}
	}
	return b.String()
}

func highlightFuzzyBg(name string, indices []int, fg lipgloss.Color, bg lipgloss.Color) string {
	baseS := lipgloss.NewStyle().Foreground(fg).Background(bg)
	if len(indices) == 0 {
		return baseS.Render(name)
	}
	hit := make(map[int]bool, len(indices))
	for _, i := range indices {
		hit[i] = true
	}
	matchS := lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Background(bg).Bold(true).Underline(true)
	var b strings.Builder
	for i, r := range name {
		if hit[i] {
			b.WriteString(matchS.Render(string(r)))
		} else {
			b.WriteString(baseS.Render(string(r)))
		}
	}
	return b.String()
}

// --- View ---

func (m model) View() string {
	if m.mode == modeSettings {
		return m.renderSettings()
	}
	if m.mode == modeTree {
		return m.renderTree()
	}
	if m.mode == modeHelp {
		return m.renderHelp()
	}

	var b strings.Builder

	pathDisplay := m.cwd
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(pathDisplay, home) {
		pathDisplay = "~" + pathDisplay[len(home):]
	}
	pathDisplay = truncatePath(pathDisplay, m.width-4)
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
			var color lipgloss.Color
			if m.config.NerdFont {
				color = entryColor(entry)
			} else {
				color = lipgloss.Color("252") // uniform default text in caret mode
			}
			icon := m.entryIconStr(entry)

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
				if !m.config.NerdFont {
					// Minimal mode: caret + plain text
					var styledName string
					if m.filter != "" {
						styledName = highlightFuzzy(name, m.matchIndices[entry.Name()], color)
					} else {
						styledName = lipgloss.NewStyle().Foreground(color).Bold(true).Render(name)
					}
					b.WriteString("> " + styledName + "\n")
				} else {
					openPill := lipgloss.NewStyle().Foreground(cursorBg).Render(m.pillOpenStr())
					closePill := lipgloss.NewStyle().Foreground(cursorBg).Render(m.pillCloseStr())
					iconStr := lipgloss.NewStyle().Foreground(color).Background(cursorBg).Render(icon)
					spacer := lipgloss.NewStyle().Background(cursorBg).Render(" ")

					var styledName string
					if m.filter != "" {
						styledName = highlightFuzzyBg(name, m.matchIndices[entry.Name()], color, cursorBg)
					} else {
						styledName = lipgloss.NewStyle().Foreground(color).Background(cursorBg).Bold(true).Render(name)
					}

					iconPart := ""
					if icon != "" {
						iconPart = iconStr + spacer
					}
					contentWidth := lipgloss.Width(iconPart) + lipgloss.Width(styledName)
					padNeeded := m.width - contentWidth - 4
					if padNeeded < 0 {
						padNeeded = 0
					}
					padStr := lipgloss.NewStyle().Background(cursorBg).Render(strings.Repeat(" ", padNeeded))

					b.WriteString(" " + openPill + iconPart + styledName + padStr + closePill + "\n")
				}
			} else {
				iconStr := lipgloss.NewStyle().Foreground(color).Render(icon)
				var coloredName string
				if m.filter != "" {
					coloredName = highlightFuzzy(name, m.matchIndices[entry.Name()], color)
				} else {
					coloredName = lipgloss.NewStyle().Foreground(color).Render(name)
				}
				if icon == "" {
					b.WriteString("  " + coloredName + "\n")
				} else {
					b.WriteString("  " + iconStr + " " + coloredName + "\n")
				}
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
		count := fmt.Sprintf("%d / %d", len(m.entries), len(m.allEntries))
		promptChar := "❯"
		if !m.config.NerdFont {
			promptChar = ">"
		}
		prompt := fmt.Sprintf("  %s %s_", promptChar, m.filter)
		hint := count + "  esc clear"
		pad := m.width - lipgloss.Width(prompt) - lipgloss.Width(hint) - 4
		if pad < 1 {
			pad = 1
		}
		promptStyle := searchInput
		if !m.config.NerdFont {
			promptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
		}
		b.WriteString(promptStyle.Render(prompt) + strings.Repeat(" ", pad) + dimStyle.Render(hint) + "\n")
	}

	// --- Status bar ---
	var statusLeft string
	switch {
	case m.filtering:
		var searchLabel string
		if m.config.NerdFont {
			searchLabel = searchPillSep.Render(m.pillOpenStr()) +
				searchPillMain.Render(" SEARCH ") +
				searchPillSep.Render(m.pillCloseStr())
		} else {
			searchLabel = dimStyle.Render("SEARCH")
		}
		statusLeft = searchLabel +
			dimStyle.Render("  ↑↓ move | → enter | ← back | / accept | enter cd | esc clear")
	default:
		var navLabel string
		if m.config.NerdFont {
			navLabel = pillSep.Render(m.pillOpenStr()) +
				pillMain.Render(" NAV ") +
				pillSep.Render(m.pillCloseStr())
		} else {
			navLabel = dimStyle.Render("NAV")
		}
		statusLeft = navLabel +
			dimStyle.Render("  hjkl | enter cd | ~ home | o open | R reveal | t tree | / find | , settings | q")
	}

	var rightParts []string

	if m.flash != "" {
		rightParts = append(rightParts, flashStyle.Render(m.flash))
	}

	infoStyle := sizeStyle
	chipStyle := hiddenBadge
	posStyleUse := posStyle
	if !m.config.NerdFont {
		infoStyle = dimStyle
		chipStyle = dimStyle
		posStyleUse = dimStyle
	}

	if total > 0 && m.cursor < len(m.entries) {
		entry := m.entries[m.cursor]
		if entry.IsDir() {
			count := countItems(filepath.Join(m.cwd, entry.Name()))
			if count == 1 {
				rightParts = append(rightParts, infoStyle.Render("1 item"))
			} else {
				rightParts = append(rightParts, infoStyle.Render(fmt.Sprintf("%d items", count)))
			}
		} else {
			if info, err := entry.Info(); err == nil {
				rightParts = append(rightParts, infoStyle.Render(humanSize(info.Size())))
			}
		}
	}

	if m.sortMode != sortName {
		rightParts = append(rightParts, chipStyle.Render("["+m.sortMode.label()+"]"))
	}

	if m.showHidden {
		rightParts = append(rightParts, chipStyle.Render("[H]"))
	}

	if total > 0 {
		rightParts = append(rightParts, posStyleUse.Render(fmt.Sprintf("%d/%d", m.cursor+1, total)))
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

func autoInstall() string {
	rcFile := detectShellRC()
	content, err := os.ReadFile(rcFile)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Sprintf("Could not read %s: %v", rcFile, err)
	}
	if strings.Contains(string(content), "NAV_WRAPPED") {
		return fmt.Sprintf("Wrapper already installed in %s", rcFile)
	}
	f, err := os.OpenFile(rcFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Sprintf("Could not write to %s: %v", rcFile, err)
	}
	defer f.Close()
	f.WriteString("\n\n" + shellWrapper + "\n")
	return fmt.Sprintf("Wrapper added to %s", rcFile)
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
	p := tea.NewProgram(setupFlowModel{}, tea.WithOutput(os.Stderr), tea.WithAltScreen())
	result, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	flow := result.(setupFlowModel)
	if flow.cancelled {
		os.Exit(0)
	}

	// Save Nerd Font preference if user chose
	var nfMsg string
	if flow.nfPicked != "" {
		cfg := loadConfig()
		cfg.NerdFont = flow.nfPicked == "yes"
		if err := saveConfig(cfg); err != nil {
			nfMsg = fmt.Sprintf("Could not save Nerd Font preference: %v", err)
		} else if cfg.NerdFont {
			nfMsg = "Nerd Font icons: ON"
		} else {
			nfMsg = "Nerd Font icons: OFF (plain text mode)"
		}
	}

	// Final summary in normal terminal (after alt screen exits)
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "  nav is set up.")
	fmt.Fprintln(os.Stderr, "")
	if flow.wrapperPicked == "automatic" && flow.wrapperMsg != "" {
		fmt.Fprintln(os.Stderr, "  Wrapper:    "+flow.wrapperMsg)
	} else if flow.wrapperPicked == "manual" {
		fmt.Fprintln(os.Stderr, "  Wrapper:    add this to "+detectShellRC()+":")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, shellWrapper)
	}
	if nfMsg != "" {
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "  Settings:   "+nfMsg)
	}
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "  Restart your terminal or run: source "+detectShellRC())
	fmt.Fprintln(os.Stderr, "")
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
	p := tea.NewProgram(m, tea.WithOutput(os.Stderr), tea.WithAltScreen(), tea.WithMouseCellMotion())

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
