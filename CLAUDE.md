# Robot Dreams — working rules

Read `README.md` for what this is and `CONTRIBUTING.md` for how to build,
test and commit. `docs/` holds the design: `docs/vision/` the product
reasoning, `docs/design-notes/` the per-area decisions that used to live
in code comments, and the rest the operator-facing guides.

## Code comments

Default: no comments. If you think you need one, do this instead:

| The comment would say… | Do this instead |
| --- | --- |
| What the code does | Rename it, or extract a function whose name says it |
| What something is for, or how its parts fit | Add a separate doc file (`docs/design-notes/<area>.md`) |
| Why a default was chosen | A named constant, or a test asserting it |
| A trap to avoid | A guard, or a test that fails on it |
| A decision and its reason | The design-notes row, or the ticket |
| What a signature takes or returns | The types already say it |

Zero comments in a file is normal.

The one exception is the public Go API under `pkg/`: each exported
identifier and each package keeps a single doc line, because that is
what `go doc` and pkg.go.dev show to importers. Tool directives
(`//go:embed`, `//go:generate`, `//go:build`, `//nolint:…`,
`// eslint-disable-next-line …`, `// @ts-expect-error`) are not
comments and stay, without prose.
