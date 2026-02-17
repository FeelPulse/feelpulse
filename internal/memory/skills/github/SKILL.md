# GitHub

Interact with GitHub using the `gh` CLI. Use `gh issue`, `gh pr`, `gh run`, and `gh api` for issues, PRs, CI runs, and advanced queries.

## Requirements

- `gh` CLI installed (`brew install gh` / `sudo apt install gh` / `sudo dnf install gh`)
- `gh auth login` completed
- `gh` added to `tools.exec.allowedCommands` in config

## Pull Requests

Create a PR:
```bash
gh pr create --title "Fix bug" --body "Description" --repo owner/repo
```

List PRs:
```bash
gh pr list --repo owner/repo
```

Check CI status on a PR:
```bash
gh pr checks 55 --repo owner/repo
```

View PR details:
```bash
gh pr view 55 --repo owner/repo
```

Merge a PR:
```bash
gh pr merge 55 --repo owner/repo --squash
```

## Issues

List issues:
```bash
gh issue list --repo owner/repo
```

Create an issue:
```bash
gh issue create --title "Bug report" --body "Details" --repo owner/repo
```

View issue:
```bash
gh issue view 42 --repo owner/repo
```

## CI/CD Runs

List recent workflow runs:
```bash
gh run list --repo owner/repo --limit 10
```

View a run and see which steps failed:
```bash
gh run view <run-id> --repo owner/repo
```

View logs for failed steps only:
```bash
gh run view <run-id> --repo owner/repo --log-failed
```

## Repository

Clone a repo:
```bash
gh repo clone owner/repo
```

Create a repo:
```bash
gh repo create my-project --public --description "My project"
```

View repo info:
```bash
gh repo view owner/repo
```

## Releases

List releases:
```bash
gh release list --repo owner/repo
```

Create a release:
```bash
gh release create v1.0.0 --title "v1.0.0" --notes "Release notes" --repo owner/repo
```

## Code Search

Search code:
```bash
gh search code "function_name" --repo owner/repo
```

## API for Advanced Queries

The `gh api` command is useful for accessing data not available through other subcommands.

Get PR with specific fields:
```bash
gh api repos/owner/repo/pulls/55 --jq '.title, .state, .user.login'
```

## JSON Output

Most commands support `--json` for structured output. Use `--jq` to filter:
```bash
gh issue list --repo owner/repo --json number,title --jq '.[] | "\(.number): \(.title)"'
```

## Tips

- Always use `--repo owner/repo` when not inside a git directory
- Use `--json` + `--jq` for structured, parseable output
- Use `gh api` for anything not covered by specific subcommands
- Add `--web` flag to open results in browser
