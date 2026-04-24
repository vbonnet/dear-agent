# Branch Protection Setup for `main`

This document describes the recommended branch protection rules for the `main` branch.
These settings must be configured manually in GitHub under **Settings > Branches > Branch protection rules**.

## Recommended Rules

### Add rule for branch: `main`

| Setting | Value |
|---------|-------|
| Require a pull request before merging | Yes |
| Required number of approvals | 1 |
| Dismiss stale pull request approvals when new commits are pushed | Yes |
| Require status checks to pass before merging | Yes |
| Require branches to be up to date before merging | Yes |
| Required status checks | `ci` (from CI workflow) |
| Require conversation resolution before merging | Yes |
| Do not allow bypassing the above settings | Yes (optional for solo maintainers) |

## How to Configure

1. Go to **https://github.com/vbonnet/dear-agent/settings/branches**
2. Click **Add branch protection rule**
3. Set **Branch name pattern** to `main`
4. Enable the settings listed above
5. Under **Require status checks to pass**, search for and add:
   - `Build & Test` (from `ci.yml`)
   - `Security Scan / govulncheck` (from `security-scan.yml`)
6. Click **Create** / **Save changes**

## Notes

- Solo maintainers may want to uncheck "Do not allow bypassing" to allow direct pushes in emergencies.
- The `Require branches to be up to date` setting prevents merging stale PRs but requires more rebasing.
