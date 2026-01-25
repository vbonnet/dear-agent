# Fixing Chezmoi Encryption Warning

## The Warning

If you see this warning when running `mcp-wizard setup`:

```
chezmoi: warning: 'encryption' not set, using age configuration.
Check if 'encryption' is correctly set as the top-level key.
```

This means your `~/.config/chezmoi/chezmoi.toml` has encryption settings but they're not configured correctly.

---

## Quick Fix (Use --force)

The wizard now uses `chezmoi apply --force` which ignores this warning. **You don't need to do anything!**

The warning doesn't prevent the file from being applied correctly.

---

## Permanent Fix (Optional)

If you want to eliminate the warning completely, fix your chezmoi config:

### Option 1: Fix the Encryption Configuration

**Edit**: `~/.config/chezmoi/chezmoi.toml`

**If you're using age encryption**, ensure `encryption` is the top-level key:

```toml
# CORRECT ✓
encryption = "age"
[age]
    identity = "~/.config/age/key.txt"
    recipient = "age1..."

# WRONG ✗
[age]
    encryption = "age"  # <- Wrong location!
    identity = "~/.config/age/key.txt"
```

**Key point**: `encryption = "age"` must be at the **top level**, not inside `[age]` section.

---

### Option 2: Remove Encryption (If Not Using It)

If you're not actually using encryption, remove the encryption settings:

**Edit**: `~/.config/chezmoi/chezmoi.toml`

**Remove or comment out** any `[age]` or encryption sections:

```toml
# [age]
#     identity = "~/.config/age/key.txt"
#     recipient = "age1..."
```

---

## Verification

After fixing, verify the warning is gone:

```bash
chezmoi apply --dry-run
```

Should run without warnings.

---

## Why This Happens

Chezmoi expects encryption configuration in a specific format. Common causes:

1. **Wrong key location**: `encryption = "age"` inside `[age]` block instead of top-level
2. **Incomplete setup**: age encryption partially configured but not finished
3. **Legacy config**: Old chezmoi config format from before v2.0

---

## Impact

**Good news**: This warning **does not prevent** chezmoi from working correctly. Files are still applied successfully.

The wizard now uses `--force` flag to bypass warnings, so you won't see this error anymore during setup.

---

**Related**: See `TROUBLESHOOTING.md` for other chezmoi issues
