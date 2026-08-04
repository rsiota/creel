# creel documentation

This folder holds creel's deeper documentation. The [main README](../README.md)
is a quickstart; the pages below are the reference.

## Guides

- [**Features**](features.md) — what creel can do, with the full detail on the
  relationship explorer (`g r`) and static ERD (`g R`).
- [**Keybindings**](keybindings.md) — every key in every panel. (Also available
  in-app with `?`.)
- [**Configuration**](configuration.md) — `config.yaml` reference: connections,
  secret/keychain storage, read-only mode, connection groups, the AI assistant,
  settings, and themes.
- [**CLI mode**](cli.md) — running queries headlessly with flags.
- [**Demo database**](../demo/README.md) — the bundled e-commerce schema used by
  the screenshots and the recorded demo.

## Internals

- [**Architecture**](architecture.md) — subsystem map and the design decisions behind the driver interface, Elm state machine, keybinding registry, and more.
- [**Command registry**](command-registry.md) — design notes on unifying `:`
  commands, keybindings, and the palette into one action layer.
- [**TUI mouse handling**](tui-mouse.md) — notes on Bubble Tea mouse-event
  routing and capture.
