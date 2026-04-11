# go-cli-template

A templated repository for a CLI written in Go.

What you get by using this repo template:

* GitHub Workflows for Build, PR checks, and Release
* Automated repository initialization via GitHub Actions
* Goreleaser configuration for cross-platform builds
  - Builds for Linux (amd64, arm64) and macOS (amd64, arm64)
  - Creates SBOM for releases
  - This does _not_ include signing (refer to [goreleaser](https://goreleaser.com/sign/) for docs)
* Community management
  - Automated labeling of issues and PRs via [jimschubert/labeler-action](https://github.com/jimschubert/labeler-action)
  - Dependabot configuration for Go modules and GitHub Actions
  - CODE_OF_CONDUCT.md and CONTRIBUTING.md templates
* A placeholder Apache 2.0 license
* Project initialization according to the owner/repository where initialized
  - Repository name becomes the application name
  - All placeholders are automatically replaced on first push to master

## Usage

1. Click "Use this template" to create a new repository
2. Clone your new repository
3. Push to main/master branch - the initialization workflow will automatically:
   - Replace all `%NAME%`, `%OWNER%`, `%PACKAGE%`, and `%REPOSITORY%` placeholders
   - Set up the proper directory structure with `cmd/<your-repo-name>/main.go`
   - Update go.mod with the correct module path
   - Clean up initialization files

## Structure

After initialization, your repository will have:

```
.
├── .github/
│   ├── workflows/
│   │   ├── build.yml           # Main build workflow
│   │   ├── pr.yml              # PR validation workflow
│   │   └── community.yml       # Issue/PR labeling
│   ├── changelog.yml           # Changelog generation config
│   ├── dependabot.yml          # Dependency updates config
│   ├── labeler.yml             # Auto-labeling rules
│   ├── CODE_OF_CONDUCT.md
│   ├── CONTRIBUTING.md
│   └── FUNDING.yml
├── cmd/
│   └── <your-app>/
│       └── main.go             # Application entrypoint
├── .gitignore
├── .goreleaser.yml
├── go.mod
├── LICENSE
└── README.md
```

## Features

- **Modern Go CLI structure** using [kong](https://github.com/alecthomas/kong) for argument parsing
- **Automated releases** via GoReleaser on version tags (v*)
- **Cross-platform builds** for Linux and macOS (amd64 and arm64)
- **Reusable workflows** from [jimschubert/.workflows](https://github.com/jimschubert/.workflows), which help reduce dependabot noise
- **SBOM generation** for release artifacts
- **Automated dependency updates** via Dependabot
- **Community engagement** via automated issue/PR labeling

## Building

Build a local distribution:

```bash
goreleaser release --snapshot --clean --skip publish
```

Or build directly with Go:

```bash
go build -o myapp ./cmd/myapp
```

## License

This project is [licensed](./LICENSE) under Apache 2.0.
