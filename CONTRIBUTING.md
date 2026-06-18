# Contributing to BuchISY

We highly appreciate feedback and contributions from the community! If you'd like to contribute to this project, please make sure to review and follow the guidelines below.

## Code of Conduct

In the interest of fostering an open and welcoming environment, please review and follow our [Code of Conduct](CODE_OF_CONDUCT.md).

## Code and Copy Reviews

All submissions, including submissions by project members, require review. We use GitHub pull requests for this purpose. Consult [GitHub Help](https://help.github.com/articles/about-pull-requests/) for more information on using pull requests.

## Report an Issue

Report all issues through [GitHub Issues](https://github.com/Bergx2/BuchISY/issues).

To help resolve your issue as quickly as possible, please include:

* **Clear and descriptive title**
* **Steps to reproduce the problem**
* **Expected vs actual behavior**
* **System information**:
  - Operating system and version (e.g., macOS 14.x, Windows 11)
  - BuchISY version (e.g., v2.1)
  - Processing mode (Claude API or Local)
* **Log files** from `~/Library/Application Support/BuchISY/logs/` (macOS) or `%APPDATA%\BuchISY\logs\` (Windows)
* **Screenshots** for UI issues

## File a Feature Request

We welcome all feature requests, whether it's to add new functionality or to improve existing features.

File your feature request through [GitHub Issues](https://github.com/Bergx2/BuchISY/issues) and include:

* **Clear description** of the suggested feature
* **Use case** - Explain why this would be useful to BuchISY users
* **Examples** or mockups if applicable
* **Alternative solutions** you've considered

## Create a Pull Request

When making pull requests to the repository, make sure to follow these guidelines for both bug fixes and new features:

* **Before creating a pull request**, file a [GitHub Issue](https://github.com/Bergx2/BuchISY/issues) so that maintainers and the community can discuss the problem and potential solutions before you spend time on an implementation.
* **In your PR's description**, link to any related issues or pull requests to give reviewers the full context of your change.
* **For commit messages**, follow the [Conventional Commits](https://www.conventionalcommits.org/) format.

**Example commit messages:**

* `feat(ui): add invoice search functionality`
* `fix(core): resolve ARM64 crash on vision extraction`
* `docs(readme): update installation guide for Windows`

### Features

Before creating pull requests for new features, first file a GitHub Issue describing the reasoning and motivation for the feature. This gives maintainers and the community the opportunity to provide feedback on your idea before implementing it.

### Bug Fixes

For bug fixes, please:

1. Create an issue describing the bug (if one doesn't exist)
2. Reference the issue in your PR: `Fixes #123`
3. Include test cases to prevent regression
4. Test on relevant platforms (macOS, Windows)

## Development Setup

### Prerequisites

* Go 1.25 or later - [Download](https://go.dev/dl/)
* Git - [Download](https://git-scm.com/downloads)
* Make (optional but recommended)
* golangci-lint (for linting) - [Install](https://golangci-lint.run/usage/install/)

### Quick Start

```bash
# Clone the repository
git clone https://github.com/Bergx2/BuchISY.git
cd BuchISY

# Install dependencies
make deps

# Build
make build

# Run tests
make test

# Format code
make fmt

# Lint
make lint
```

### Project Structure

```
internal/
├── ui/          # Fyne UI layer
├── core/        # Business logic
├── db/          # SQLite database
├── anthropic/   # Claude API integration
├── i18n/        # Internationalization
└── logging/     # Structured logging
```

## Code Style

* Follow [Effective Go](https://go.dev/doc/effective_go)
* Run `make fmt` before committing
* Pass `make lint` checks
* Add comments for exported functions
* Use meaningful variable names
* Update i18n files (`assets/i18n/`) for new UI strings

### Translations

When adding new UI strings:

1. Add German translation to `assets/i18n/de.toml`
2. Add English translation to `assets/i18n/en.toml`
3. Use descriptive keys: `field.company` not `label1`

## Testing

* Add tests for new features
* Run `make test` to ensure all tests pass
* Add test cases for bug fixes to prevent regression
* Test on both macOS and Windows if possible

## License

By contributing to BuchISY, you agree that your contributions will be licensed under the [MIT License](LICENSE).

## Need Help?

* **Questions**: Open an [issue](https://github.com/Bergx2/BuchISY/issues)
* **Security issues**: Email [info@bergx2.de](mailto:info@bergx2.de) directly
* **Enterprise support**: Contact [info@bergx2.de](mailto:info@bergx2.de)

## Thank You!

Your contributions make BuchISY better for everyone. We appreciate your time and effort!

---

**BuchISY** - Open Source Invoice Management by [Bergx2 GmbH](https://www.bergx2.de)

🌐 [www.buchisy.de](https://www.buchisy.de) | 💻 [GitHub](https://github.com/Bergx2/BuchISY)
