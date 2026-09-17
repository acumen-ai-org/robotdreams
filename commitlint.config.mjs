// Commit message rules (CONTRIBUTING.md, "Commit messages"). Checked on
// every pull request by the commitlint job in .github/workflows/ci.yml;
// release-please derives the changelog and the version bump from them.
//
// The rules are @commitlint/config-conventional's, written out rather
// than extended so the file resolves without a node_modules next to it:
// the check runs with a bare `npx -p @commitlint/cli commitlint`.
export default {
  rules: {
    "body-leading-blank": [1, "always"],
    "body-max-line-length": [2, "always", 72],
    "footer-leading-blank": [1, "always"],
    "footer-max-line-length": [2, "always", 100],
    "header-max-length": [2, "always", 72],
    "header-trim": [2, "always"],
    "subject-case": [2, "never", ["sentence-case", "start-case", "pascal-case", "upper-case"]],
    "subject-empty": [2, "never"],
    "subject-full-stop": [2, "never", "."],
    "type-case": [2, "always", "lower-case"],
    "type-empty": [2, "never"],
    "type-enum": [
      2,
      "always",
      ["feat", "fix", "perf", "refactor", "docs", "test", "build", "ci", "chore", "revert"],
    ],
  },
};
