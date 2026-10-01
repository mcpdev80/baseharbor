# Global CLI options

These options are processed before command dispatch.

| Option | Meaning |
| --- | --- |
| `--target NAME` / `--target=NAME` | Override Target resolution |
| `-q`, `--quiet`, `--silent` | Concise output |
| `-v`, `--verbose` | Diagnostic output |
| `--no-color` | Disable color |
| `--plain` | Plain output |
| `--no-input`, `--non-interactive` | Never prompt for user input |
| `--version` | Show BaseHarbor version |

`--quiet` and `--verbose` are mutually exclusive.

## Automation rule

For automation, prefer explicit Target/environment selection where ambiguity matters, non-interactive mode, and structured JSON/MCP results rather than parsing human text.
