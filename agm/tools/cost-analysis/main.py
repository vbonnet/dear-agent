#!/usr/bin/env python3
"""
Claude Code Cost Analytics Report
Scans all conversation logs and produces detailed cost breakdowns.
"""
import glob
import json
import os
import sys
from collections import defaultdict
from datetime import datetime, timezone, timedelta

# ── Pricing (per million tokens) ──────────────────────────────────────────────
PRICING = {
    # model_prefix: (input, output, cache_read, cache_write)
    'opus':   (15.00,  75.00,  1.50, 18.75),
    'sonnet': ( 3.00,  15.00,  0.30,  3.75),
    'haiku':  ( 0.80,   4.00,  0.08,  1.00),
}

def classify_model(model_str):
    """Return model family key from model string."""
    if not model_str:
        return None
    m = model_str.lower()
    if 'opus' in m:
        return 'opus'
    if 'sonnet' in m:
        return 'sonnet'
    if 'haiku' in m:
        return 'haiku'
    return None

def compute_cost(usage, model_family):
    """Compute USD cost from a usage dict and model family."""
    if not model_family or model_family not in PRICING:
        return 0.0
    inp_price, out_price, cr_price, cw_price = PRICING[model_family]
    input_tokens = usage.get('input_tokens', 0) or 0
    output_tokens = usage.get('output_tokens', 0) or 0
    cache_read = usage.get('cache_read_input_tokens', 0) or 0
    cache_write = usage.get('cache_creation_input_tokens', 0) or 0
    cost = (
        input_tokens * inp_price / 1_000_000
        + output_tokens * out_price / 1_000_000
        + cache_read * cr_price / 1_000_000
        + cache_write * cw_price / 1_000_000
    )
    return cost

def parse_timestamp(ts_str):
    """Parse ISO timestamp string to datetime (UTC)."""
    if not ts_str:
        return None
    try:
        # Handle various formats
        ts_str = ts_str.replace('Z', '+00:00')
        return datetime.fromisoformat(ts_str).astimezone(timezone.utc)
    except Exception:
        return None

# ── Data collection ───────────────────────────────────────────────────────────
CUTOFF = datetime(2025, 1, 1, tzinfo=timezone.utc)

# Accumulators
daily_cost = defaultdict(float)           # date_str -> cost
daily_by_model = defaultdict(lambda: defaultdict(float))  # date -> {family -> cost}
def _new_session():
    return {
        'cost': 0.0, 'models': set(), 'first_ts': None, 'last_ts': None,
        'input_tokens': 0, 'output_tokens': 0, 'cache_read': 0, 'cache_write': 0,
        'file': '',
        'model_costs': defaultdict(float),
        'model_tokens': defaultdict(lambda: defaultdict(int)),
    }
session_data = defaultdict(_new_session)
hourly_cost = defaultdict(float)          # hour (0-23) -> cost
total_tokens = {'input': 0, 'output': 0, 'cache_read': 0, 'cache_write': 0}
model_family_cost = defaultdict(float)    # family -> cost
model_family_tokens = defaultdict(lambda: defaultdict(int))

def process_usage(usage, model_str, timestamp_str, session_id, filepath):
    """Process a single usage record."""
    family = classify_model(model_str)
    if not family:
        return
    cost = compute_cost(usage, family)
    ts = parse_timestamp(timestamp_str)
    if not ts or ts < CUTOFF:
        return

    date_str = ts.strftime('%Y-%m-%d')
    daily_cost[date_str] += cost
    daily_by_model[date_str][family] += cost
    hourly_cost[ts.hour] += cost
    model_family_cost[family] += cost

    input_tokens = usage.get('input_tokens', 0) or 0
    output_tokens = usage.get('output_tokens', 0) or 0
    cache_read = usage.get('cache_read_input_tokens', 0) or 0
    cache_write = usage.get('cache_creation_input_tokens', 0) or 0

    total_tokens['input'] += input_tokens
    total_tokens['output'] += output_tokens
    total_tokens['cache_read'] += cache_read
    total_tokens['cache_write'] += cache_write
    model_family_tokens[family]['input'] += input_tokens
    model_family_tokens[family]['output'] += output_tokens
    model_family_tokens[family]['cache_read'] += cache_read
    model_family_tokens[family]['cache_write'] += cache_write

    sd = session_data[session_id]
    sd['cost'] += cost
    sd['models'].add(family)
    sd['input_tokens'] += input_tokens
    sd['output_tokens'] += output_tokens
    sd['cache_read'] += cache_read
    sd['cache_write'] += cache_write
    sd['file'] = filepath
    sd['model_costs'][family] += cost
    sd['model_tokens'][family]['input'] += input_tokens
    sd['model_tokens'][family]['output'] += output_tokens
    sd['model_tokens'][family]['cache_read'] += cache_read
    sd['model_tokens'][family]['cache_write'] += cache_write
    if sd['first_ts'] is None or ts < sd['first_ts']:
        sd['first_ts'] = ts
    if sd['last_ts'] is None or ts > sd['last_ts']:
        sd['last_ts'] = ts

def process_entry(entry, session_id, filepath):
    """Process a single JSONL entry."""
    etype = entry.get('type')
    ts_str = entry.get('timestamp')

    if etype == 'assistant':
        msg = entry.get('message', {})
        if isinstance(msg, dict) and msg.get('usage'):
            process_usage(msg['usage'], msg.get('model'), ts_str, session_id, filepath)

    elif etype == 'progress':
        data = entry.get('data', {})
        if isinstance(data, dict):
            msg = data.get('message', {})
            if isinstance(msg, dict) and msg.get('usage'):
                process_usage(msg['usage'], msg.get('model'), ts_str, session_id, filepath)

# ── Scan all JSONL files ──────────────────────────────────────────────────────
print("\033[1;36m" + "=" * 100 + "\033[0m")
print("\033[1;36m  CLAUDE CODE COST ANALYTICS REPORT\033[0m")
print("\033[1;36m  " + "=" * 98 + "\033[0m")
print()

jsonl_files = glob.glob(os.path.expanduser('~/.claude/projects/**/*.jsonl'), recursive=True)
# Also check for .jsonl.backup files
backup_files = glob.glob(os.path.expanduser('~/.claude/projects/**/*.jsonl.backup-*'), recursive=True)

# Deduplicate: if a session has both a .jsonl and a .jsonl.backup, prefer the .jsonl
# The backup files likely have older data that was superseded
# Actually, we should process both as they may cover different time ranges
# But we need to avoid double-counting. Backups are renamed originals.
# Let's just use .jsonl files since backups are renamed versions.

total_files = len(jsonl_files)
print(f"  Scanning {total_files} JSONL files...", end='', flush=True)

files_processed = 0
lines_processed = 0
errors = 0

# Track which sessions we've seen to handle subagent symlinks
seen_files = set()

for filepath in jsonl_files:
    # Resolve symlinks to avoid double counting
    real_path = os.path.realpath(filepath)
    if real_path in seen_files:
        continue
    seen_files.add(real_path)

    # Extract session ID from filename
    basename = os.path.basename(filepath)
    session_id = basename.replace('.jsonl', '')

    try:
        with open(filepath, 'r', errors='replace') as fh:
            for line in fh:
                line = line.strip()
                if not line:
                    continue
                try:
                    entry = json.loads(line)
                    process_entry(entry, session_id, filepath)
                    lines_processed += 1
                except (json.JSONDecodeError, Exception):
                    errors += 1
    except (OSError, IOError):
        errors += 1

    files_processed += 1
    if files_processed % 500 == 0:
        print(f"\r  Scanning {total_files} JSONL files... {files_processed}/{total_files}", end='', flush=True)

print(f"\r  Scanned {files_processed} files ({len(seen_files)} unique), {lines_processed:,} entries, {errors} errors" + " " * 20)
print()

# ── Helper formatting ─────────────────────────────────────────────────────────
def fmt_usd(amount):
    if amount >= 1.0:
        return f"${amount:,.2f}"
    return f"${amount:,.4f}"

def fmt_tokens(count):
    if count >= 1_000_000:
        return f"{count/1_000_000:.1f}M"
    if count >= 1_000:
        return f"{count/1_000:.1f}K"
    return str(count)

def bar_chart(value, max_value, width=50):
    if max_value == 0:
        return ""
    filled = int(value / max_value * width)
    return "\033[32m" + "\u2588" * filled + "\033[0m" + "\u2591" * (width - filled)

# ── Report Section A: Daily Cost ──────────────────────────────────────────────
print("\033[1;33m" + "\u2500" * 100 + "\033[0m")
print("\033[1;33m  A. DAILY COST (since Jan 1, 2025)\033[0m")
print("\033[1;33m" + "\u2500" * 100 + "\033[0m")

if daily_cost:
    sorted_days = sorted(daily_cost.keys())
    max_daily = max(daily_cost.values())
    grand_total = sum(daily_cost.values())

    # Show all days with cost
    for day in sorted_days:
        cost = daily_cost[day]
        bar = bar_chart(cost, max_daily, 45)
        # Color code by amount
        if cost >= 50:
            color = "\033[1;31m"  # bright red
        elif cost >= 20:
            color = "\033[31m"    # red
        elif cost >= 10:
            color = "\033[33m"    # yellow
        else:
            color = "\033[0m"     # default
        print(f"  {day}  {bar} {color}{fmt_usd(cost):>10}\033[0m")

    print()
    print(f"  \033[1mGrand Total: {fmt_usd(grand_total)}\033[0m")
    print(f"  \033[1mDays with activity: {len(sorted_days)}\033[0m")
    if sorted_days:
        total_span = (datetime.strptime(sorted_days[-1], '%Y-%m-%d') -
                      datetime.strptime(sorted_days[0], '%Y-%m-%d')).days + 1
        print(f"  \033[1mAvg per active day: {fmt_usd(grand_total / len(sorted_days))}\033[0m")
        print(f"  \033[1mAvg per calendar day: {fmt_usd(grand_total / max(total_span, 1))}\033[0m")
else:
    print("  No data found.")
print()

# ── Report Section B: Weekly Summary ──────────────────────────────────────────
print("\033[1;33m" + "\u2500" * 100 + "\033[0m")
print("\033[1;33m  B. WEEKLY COST SUMMARY\033[0m")
print("\033[1;33m" + "\u2500" * 100 + "\033[0m")

weekly_cost = defaultdict(float)
weekly_days = defaultdict(int)
for day_str, cost in daily_cost.items():
    dt = datetime.strptime(day_str, '%Y-%m-%d')
    iso_year, iso_week, _ = dt.isocalendar()
    week_key = f"{iso_year}-W{iso_week:02d}"
    # Find Monday of this week for display
    monday = dt - timedelta(days=dt.weekday())
    week_label = f"{week_key} ({monday.strftime('%b %d')})"
    weekly_cost[week_label] += cost
    weekly_days[week_label] += 1

if weekly_cost:
    max_weekly = max(weekly_cost.values())
    print(f"  {'Week':<22} {'Total':>10}  {'Avg/Day':>10}  {'Active':>6}  Chart")
    print(f"  {'\u2500'*22} {'\u2500'*10}  {'\u2500'*10}  {'\u2500'*6}  {'\u2500'*40}")
    for week in sorted(weekly_cost.keys()):
        cost = weekly_cost[week]
        active = weekly_days[week]
        avg = cost / active
        bar = bar_chart(cost, max_weekly, 35)
        print(f"  {week:<22} {fmt_usd(cost):>10}  {fmt_usd(avg):>10}  {active:>6}  {bar}")
print()

# ── Report Section C: Monthly Summary ─────────────────────────────────────────
print("\033[1;33m" + "\u2500" * 100 + "\033[0m")
print("\033[1;33m  C. MONTHLY COST SUMMARY\033[0m")
print("\033[1;33m" + "\u2500" * 100 + "\033[0m")

monthly_cost = defaultdict(float)
monthly_sessions = defaultdict(set)
for day_str, cost in daily_cost.items():
    month_key = day_str[:7]  # YYYY-MM
    monthly_cost[month_key] += cost

# Count sessions per month
for sid, sd in session_data.items():
    if sd['first_ts']:
        month_key = sd['first_ts'].strftime('%Y-%m')
        monthly_sessions[month_key].add(sid)

if monthly_cost:
    max_monthly = max(monthly_cost.values())
    print(f"  {'Month':<10} {'Total':>12}  {'Sessions':>8}  {'Avg/Session':>12}  {'Avg/Day':>10}  Chart")
    print(f"  {'\u2500'*10} {'\u2500'*12}  {'\u2500'*8}  {'\u2500'*12}  {'\u2500'*10}  {'\u2500'*35}")

    import calendar
    for month in sorted(monthly_cost.keys()):
        cost = monthly_cost[month]
        sess = len(monthly_sessions.get(month, set()))
        year, mon = int(month[:4]), int(month[5:7])
        days_in_month = calendar.monthrange(year, mon)[1]
        avg_session = cost / max(sess, 1)
        avg_day = cost / days_in_month
        bar = bar_chart(cost, max_monthly, 30)
        month_name = datetime(year, mon, 1).strftime('%Y %b')
        print(f"  {month_name:<10} {fmt_usd(cost):>12}  {sess:>8}  {fmt_usd(avg_session):>12}  {fmt_usd(avg_day):>10}  {bar}")
print()

# ── Report Section D: Cost by Model Family ────────────────────────────────────
print("\033[1;33m" + "\u2500" * 100 + "\033[0m")
print("\033[1;33m  D. COST BY MODEL FAMILY\033[0m")
print("\033[1;33m" + "\u2500" * 100 + "\033[0m")

total_cost = sum(model_family_cost.values())
if model_family_cost:
    max_model_cost = max(model_family_cost.values())
    colors = {'opus': '\033[35m', 'sonnet': '\033[34m', 'haiku': '\033[36m'}
    labels = {'opus': 'Opus (claude-opus-4*)', 'sonnet': 'Sonnet (claude-sonnet-4*)', 'haiku': 'Haiku (claude-haiku-4*)'}

    print(f"  {'Model':<30} {'Cost':>12}  {'%':>7}  {'Input Tok':>10}  {'Output Tok':>10}  {'Cache R':>10}  {'Cache W':>10}")
    print(f"  {'\u2500'*30} {'\u2500'*12}  {'\u2500'*7}  {'\u2500'*10}  {'\u2500'*10}  {'\u2500'*10}  {'\u2500'*10}")

    for family in ['opus', 'sonnet', 'haiku']:
        if family not in model_family_cost:
            continue
        cost = model_family_cost[family]
        pct = cost / total_cost * 100 if total_cost > 0 else 0
        color = colors.get(family, '')
        label = labels.get(family, family)
        toks = model_family_tokens[family]
        bar = bar_chart(cost, max_model_cost, 20)
        print(f"  {color}{label:<30}\033[0m {fmt_usd(cost):>12}  {pct:>6.1f}%  "
              f"{fmt_tokens(toks['input']):>10}  {fmt_tokens(toks['output']):>10}  "
              f"{fmt_tokens(toks['cache_read']):>10}  {fmt_tokens(toks['cache_write']):>10}")
    print(f"  {'\u2500'*30} {'\u2500'*12}")
    print(f"  {'TOTAL':<30} \033[1m{fmt_usd(total_cost):>12}\033[0m")
print()

# ── Report Section E: Top 10 Most Expensive Sessions ──────────────────────────
print("\033[1;33m" + "\u2500" * 100 + "\033[0m")
print("\033[1;33m  E. TOP 10 MOST EXPENSIVE SESSIONS\033[0m")
print("\033[1;33m" + "\u2500" * 100 + "\033[0m")

sorted_sessions = sorted(session_data.items(), key=lambda x: x[1]['cost'], reverse=True)

print(f"  {'#':<3} {'Session ID':<40} {'Cost':>10}  {'Model(s)':<15}  {'Duration':<12}  {'Project'}")
print(f"  {'\u2500'*3} {'\u2500'*40} {'\u2500'*10}  {'\u2500'*15}  {'\u2500'*12}  {'\u2500'*25}")

for i, (sid, sd) in enumerate(sorted_sessions[:10], 1):
    models = ', '.join(sorted(sd['models']))
    if sd['first_ts'] and sd['last_ts']:
        duration = sd['last_ts'] - sd['first_ts']
        total_secs = int(duration.total_seconds())
        if total_secs >= 3600:
            dur_str = f"{total_secs // 3600}h {(total_secs % 3600) // 60}m"
        elif total_secs >= 60:
            dur_str = f"{total_secs // 60}m {total_secs % 60}s"
        else:
            dur_str = f"{total_secs}s"
    else:
        dur_str = "N/A"

    # Extract project from filepath
    fpath = sd['file']
    parts = fpath.split('/projects/')
    if len(parts) > 1:
        project = parts[1].split('/')[0][:25]
    else:
        project = "?"

    print(f"  {i:<3} {sid:<40} {fmt_usd(sd['cost']):>10}  {models:<15}  {dur_str:<12}  {project}")

print()

def fmt_duration(total_secs):
    """Format seconds into a human-readable duration string."""
    if total_secs >= 3600:
        return f"{total_secs // 3600}h {(total_secs % 3600) // 60}m"
    elif total_secs >= 60:
        return f"{total_secs // 60}m {total_secs % 60}s"
    else:
        return f"{total_secs}s"

# ── Report Section E2: Per-Session Model Breakdown ───────────────────────────
print("\033[1;33m" + "\u2500" * 100 + "\033[0m")
print("\033[1;33m  E2. PER-SESSION MODEL BREAKDOWN (top 10)\033[0m")
print("\033[1;33m" + "\u2500" * 100 + "\033[0m")

for i, (sid, sd) in enumerate(sorted_sessions[:10], 1):
    print(f"  Session: {sid[:40]}    Total: {fmt_usd(sd['cost'])}")
    for family in ['opus', 'sonnet', 'haiku']:
        mc = sd['model_costs'].get(family, 0.0)
        if mc <= 0:
            continue
        pct = mc / sd['cost'] * 100 if sd['cost'] > 0 else 0
        mt = sd['model_tokens'].get(family, {})
        label = family.capitalize()
        print(f"    {label:<8} {fmt_usd(mc):>10}  ({pct:>5.1f}%)   "
              f"Input: {fmt_tokens(mt.get('input', 0)):>6}  "
              f"Output: {fmt_tokens(mt.get('output', 0)):>6}  "
              f"Cache-R: {fmt_tokens(mt.get('cache_read', 0)):>6}")
    print()
print()

# ── Report Section E3: Burn Rate ─────────────────────────────────────────────
print("\033[1;33m" + "\u2500" * 100 + "\033[0m")
print("\033[1;33m  E3. BURN RATE (top 10 sessions)\033[0m")
print("\033[1;33m" + "\u2500" * 100 + "\033[0m")

print(f"  {'Session':<40} {'Cost':>10}  {'Duration':>12}  {'$/hour':>12}  {'Flag'}")
print(f"  {'\u2500'*40} {'\u2500'*10}  {'\u2500'*12}  {'\u2500'*12}  {'\u2500'*10}")

for i, (sid, sd) in enumerate(sorted_sessions[:10], 1):
    if sd['first_ts'] and sd['last_ts']:
        duration = sd['last_ts'] - sd['first_ts']
        total_secs = int(duration.total_seconds())
        dur_str = fmt_duration(total_secs)
        if total_secs > 0:
            per_hour = sd['cost'] / (total_secs / 3600.0)
        else:
            per_hour = sd['cost'] * 3600.0 if sd['cost'] > 0 else 0.0
    else:
        dur_str = "N/A"
        per_hour = 0.0

    flag = ""
    if per_hour > 100:
        flag = "\033[1;31m<- RUNAWAY\033[0m"

    print(f"  {sid:<40} {fmt_usd(sd['cost']):>10}  {dur_str:>12}  "
          f"{fmt_usd(per_hour) + '/hr':>12}  {flag}")

print()

# ── Report Section E4: Per-Project Breakdown ─────────────────────────────────
print("\033[1;33m" + "\u2500" * 100 + "\033[0m")
print("\033[1;33m  E4. PER-PROJECT BREAKDOWN\033[0m")
print("\033[1;33m" + "\u2500" * 100 + "\033[0m")

project_data = defaultdict(lambda: {'sessions': 0, 'cost': 0.0})
for sid, sd in session_data.items():
    fpath = sd['file']
    parts = fpath.split('/projects/')
    if len(parts) > 1:
        proj = parts[1].split('/')[0]
    else:
        proj = "(unknown)"
    project_data[proj]['sessions'] += 1
    project_data[proj]['cost'] += sd['cost']

sorted_projects = sorted(project_data.items(), key=lambda x: x[1]['cost'], reverse=True)

print(f"  {'Project':<40} {'Sessions':>8}  {'Cost':>12}  {'Avg/Session':>12}")
print(f"  {'\u2500'*40} {'\u2500'*8}  {'\u2500'*12}  {'\u2500'*12}")

for proj, pd in sorted_projects:
    avg = pd['cost'] / max(pd['sessions'], 1)
    print(f"  {proj:<40} {pd['sessions']:>8}  {fmt_usd(pd['cost']):>12}  {fmt_usd(avg):>12}")

print()

# ── Report Section F: Token Usage Breakdown ───────────────────────────────────
print("\033[1;33m" + "\u2500" * 100 + "\033[0m")
print("\033[1;33m  F. TOKEN USAGE BREAKDOWN\033[0m")
print("\033[1;33m" + "\u2500" * 100 + "\033[0m")

ti = total_tokens['input']
to = total_tokens['output']
cr = total_tokens['cache_read']
cw = total_tokens['cache_write']
total_input_context = ti + cr + cw  # All tokens that contribute to input

print(f"  Input tokens (fresh):       {fmt_tokens(ti):>12}  ({ti:>15,})")
print(f"  Output tokens:              {fmt_tokens(to):>12}  ({to:>15,})")
print(f"  Cache read tokens:          {fmt_tokens(cr):>12}  ({cr:>15,})")
print(f"  Cache creation tokens:      {fmt_tokens(cw):>12}  ({cw:>15,})")
print()

if total_input_context > 0:
    cache_hit_rate = cr / total_input_context * 100
    cache_write_rate = cw / total_input_context * 100
    fresh_rate = ti / total_input_context * 100
    print(f"  Cache efficiency (of all input context):")
    print(f"    Cache hits (read):     {cache_hit_rate:>6.1f}%  {bar_chart(cr, total_input_context, 40)}")
    print(f"    Cache writes:          {cache_write_rate:>6.1f}%  {bar_chart(cw, total_input_context, 40)}")
    print(f"    Fresh input:           {fresh_rate:>6.1f}%  {bar_chart(ti, total_input_context, 40)}")

    # Cost savings from caching
    # Without caching, all cache_read tokens would have been charged at full input price
    # Calculate savings per model family
    total_saved = 0.0
    for family in ['opus', 'sonnet', 'haiku']:
        if family not in model_family_tokens:
            continue
        inp_price = PRICING[family][0]
        cr_price = PRICING[family][2]
        cr_tok = model_family_tokens[family]['cache_read']
        saved = cr_tok * (inp_price - cr_price) / 1_000_000
        total_saved += saved
    print(f"\n  \033[1;32mEstimated cache savings: {fmt_usd(total_saved)}\033[0m")
    if total_cost > 0:
        print(f"  Without caching would have cost: ~{fmt_usd(total_cost + total_saved)}")
        print(f"  Cache discount: {total_saved / (total_cost + total_saved) * 100:.1f}%")
print()

# ── Report Section G: Cost Trend ──────────────────────────────────────────────
print("\033[1;33m" + "\u2500" * 100 + "\033[0m")
print("\033[1;33m  G. COST TREND (last 7 days vs previous 7 days)\033[0m")
print("\033[1;33m" + "\u2500" * 100 + "\033[0m")

today = datetime(2026, 3, 24, tzinfo=timezone.utc)  # Current date from context
last_7 = 0.0
prev_7 = 0.0
for i in range(7):
    day = (today - timedelta(days=i)).strftime('%Y-%m-%d')
    last_7 += daily_cost.get(day, 0)
for i in range(7, 14):
    day = (today - timedelta(days=i)).strftime('%Y-%m-%d')
    prev_7 += daily_cost.get(day, 0)

print(f"  Last 7 days (Mar 18-24):     {fmt_usd(last_7):>12}")
print(f"  Previous 7 days (Mar 11-17): {fmt_usd(prev_7):>12}")

if prev_7 > 0:
    change = (last_7 - prev_7) / prev_7 * 100
    direction = "\033[31m\u25b2" if change > 0 else "\033[32m\u25bc"
    print(f"  Change: {direction} {abs(change):.1f}%\033[0m")
elif last_7 > 0:
    print(f"  Change: \033[31m\u25b2 New spending (no prior week data)\033[0m")
else:
    print(f"  Change: No activity in either period")

# Also show last 30 vs previous 30
last_30 = sum(daily_cost.get((today - timedelta(days=i)).strftime('%Y-%m-%d'), 0) for i in range(30))
prev_30 = sum(daily_cost.get((today - timedelta(days=i)).strftime('%Y-%m-%d'), 0) for i in range(30, 60))
print()
print(f"  Last 30 days:     {fmt_usd(last_30):>12}")
print(f"  Previous 30 days: {fmt_usd(prev_30):>12}")
if prev_30 > 0:
    change30 = (last_30 - prev_30) / prev_30 * 100
    direction = "\033[31m\u25b2" if change30 > 0 else "\033[32m\u25bc"
    print(f"  Change: {direction} {abs(change30):.1f}%\033[0m")
print()

# ── Report Section H: Hourly Distribution ─────────────────────────────────────
print("\033[1;33m" + "\u2500" * 100 + "\033[0m")
print("\033[1;33m  H. HOURLY DISTRIBUTION (UTC)\033[0m")
print("\033[1;33m" + "\u2500" * 100 + "\033[0m")

if hourly_cost:
    max_hourly = max(hourly_cost.values())
    for hour in range(24):
        cost = hourly_cost.get(hour, 0)
        bar = bar_chart(cost, max_hourly, 50)
        marker = " **" if cost == max_hourly and cost > 0 else ""
        print(f"  {hour:02d}:00  {bar} {fmt_usd(cost):>10}{marker}")

    peak_hour = max(hourly_cost, key=hourly_cost.get)
    print(f"\n  Peak hour: \033[1m{peak_hour:02d}:00 UTC\033[0m ({fmt_usd(hourly_cost[peak_hour])})")
print()

# ── Summary ───────────────────────────────────────────────────────────────────
print("\033[1;36m" + "=" * 100 + "\033[0m")
print("\033[1;36m  SUMMARY\033[0m")
print("\033[1;36m" + "=" * 100 + "\033[0m")
print(f"  Total spend since Jan 1, 2025:  \033[1m{fmt_usd(total_cost)}\033[0m")
print(f"  Total sessions:                 \033[1m{len(session_data):,}\033[0m")
print(f"  Active days:                    \033[1m{len(daily_cost)}\033[0m")
if daily_cost:
    print(f"  Most expensive day:             \033[1m{max(daily_cost, key=daily_cost.get)} ({fmt_usd(max(daily_cost.values()))})\033[0m")
    print(f"  Most expensive session:         \033[1m{sorted_sessions[0][0][:36]}... ({fmt_usd(sorted_sessions[0][1]['cost'])})\033[0m")
print(f"  Total tokens processed:         \033[1m{fmt_tokens(ti + to + cr + cw)}\033[0m")
print("\033[1;36m" + "=" * 100 + "\033[0m")
