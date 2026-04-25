# nav

A tiny terminal file navigator. Browse folders with vim keys, press enter, and your shell `cd`s into it.

## Install

### Homebrew

```sh
brew install TheGentleTurtle/tap/nav
```

### Go

```sh
go install github.com/TheGentleTurtle/nav@latest
```

### From source

```sh
git clone https://github.com/TheGentleTurtle/nav.git
cd nav
go build -o nav .
```

## Shell setup

The first time you run `nav`, it will set up a small shell wrapper automatically. This wrapper is what lets `nav` change your working directory — without it, `nav` can only display paths.

You can also run setup manually:

```sh
nav --setup
```

The wrapper it adds to your `~/.zshrc` or `~/.bashrc`:

```sh
# --- nav - terminal directory navigator ---
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
# --- end nav ---
```

Then restart your shell or run `source ~/.zshrc`.

## Commands

```sh
nav             # open navigator — pick a folder and cd into it
nav --setup     # install/show shell wrapper setup
nav --help      # show help and keybindings
nav --version   # print version
nav --uninstall # remove wrapper and uninstall Homebrew formula
```

## Keys

| Key | Action |
|-----|--------|
| `hjkl` / `↑↓←→` | Navigate |
| `l` / `→` | Enter folder |
| `h` / `←` | Go back |
| `Enter` | **cd into selected folder and exit** |
| `o` | Open selected item in the default app |
| `c` | Copy selected path to clipboard |
| `~` | Jump to home directory |
| `[.]` | Toggle hidden files |
| `/` | Search / accept search results |
| `Esc` | Clear search |
| `q` | Quit without changing directory |

## License

[CC BY-NC 4.0](https://creativecommons.org/licenses/by-nc/4.0/) — free for personal use, no commercial use.