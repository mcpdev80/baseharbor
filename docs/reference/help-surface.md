# Golden CLI help surface

Card 0428-306 (bug-hunt H-V36) found the CLI help surface consistent and added
a regression guard instead of a repair. `cmd/baha/testdata/cli-help-golden.txt`
pins the exact rendered `--help` text of every public command.

Drift fails the ordinary CLI package tests in CI. Review a golden diff like any
other product surface change, then regenerate:

```bash
BASEHARBOR_UPDATE_MACHINE_DOCS=1 go test ./cmd/baha -run TestCLIHelpGoldenSurface
```

The golden renders the command tree outside an application repository at
`COLUMNS=100`. Live help routes stay authoritative: run `baha COMMAND --help`
for the command you operate on. See the [command index](../cli/command-index.md)
for the command inventory and [global options](../cli/global-options.md).
