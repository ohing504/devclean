#!/bin/bash
# Creates a fake workspace for demo screenshots/GIFs
#
# WARNING: This creates ~4 GB of dummy files at ~/devclean-demo
# Cleanup: rm -rf ~/devclean-demo
#
# Usage: ./demo/setup.sh

set -e

echo "⚠ This will create ~4 GB of dummy files at ~/devclean-demo"
echo "  Cleanup with: rm -rf ~/devclean-demo"
echo ""

DEMO_DIR="$HOME/devclean-demo"
rm -rf "$DEMO_DIR"
mkdir -p "$DEMO_DIR"

# --- Node.js projects ---

# 1. e-commerce (large, active — 2 days ago)
mkdir -p "$DEMO_DIR/e-commerce/node_modules/react"
mkdir -p "$DEMO_DIR/e-commerce/.next/cache"
dd if=/dev/zero of="$DEMO_DIR/e-commerce/node_modules/react/index.js" bs=1024 count=512000 2>/dev/null
dd if=/dev/zero of="$DEMO_DIR/e-commerce/.next/cache/data.json" bs=1024 count=256000 2>/dev/null
echo '{}' > "$DEMO_DIR/e-commerce/package.json"
git -C "$DEMO_DIR/e-commerce" init -q
git -C "$DEMO_DIR/e-commerce" add -A
git -C "$DEMO_DIR/e-commerce" -c user.email="demo@test.com" -c user.name="demo" commit -q -m "init"
touch -t $(date -v-2d +%Y%m%d%H%M) "$DEMO_DIR/e-commerce"

# 2. blog-app (medium, stale — 2 months ago)
mkdir -p "$DEMO_DIR/blog-app/node_modules/next"
mkdir -p "$DEMO_DIR/blog-app/.next/static"
dd if=/dev/zero of="$DEMO_DIR/blog-app/node_modules/next/index.js" bs=1024 count=384000 2>/dev/null
dd if=/dev/zero of="$DEMO_DIR/blog-app/.next/static/chunks.js" bs=1024 count=128000 2>/dev/null
echo '{}' > "$DEMO_DIR/blog-app/package.json"
git -C "$DEMO_DIR/blog-app" init -q
git -C "$DEMO_DIR/blog-app" add -A
GIT_COMMITTER_DATE="2026-01-15T10:00:00" git -C "$DEMO_DIR/blog-app" -c user.email="demo@test.com" -c user.name="demo" commit -q -m "init" --date="2026-01-15T10:00:00"
touch -t 202601151000 "$DEMO_DIR/blog-app"
touch -t 202601151000 "$DEMO_DIR/blog-app/node_modules"
touch -t 202601151000 "$DEMO_DIR/blog-app/.next"

# 3. my-turborepo (monorepo, dormant — 5 months ago)
mkdir -p "$DEMO_DIR/my-turborepo/node_modules/turbo"
mkdir -p "$DEMO_DIR/my-turborepo/.turbo"
mkdir -p "$DEMO_DIR/my-turborepo/apps/web/.next/cache"
mkdir -p "$DEMO_DIR/my-turborepo/apps/web/node_modules"
mkdir -p "$DEMO_DIR/my-turborepo/apps/admin/.next/cache"
mkdir -p "$DEMO_DIR/my-turborepo/packages/ui/node_modules"
dd if=/dev/zero of="$DEMO_DIR/my-turborepo/node_modules/turbo/index.js" bs=1024 count=640000 2>/dev/null
dd if=/dev/zero of="$DEMO_DIR/my-turborepo/.turbo/cache.json" bs=1024 count=128000 2>/dev/null
dd if=/dev/zero of="$DEMO_DIR/my-turborepo/apps/web/.next/cache/data.json" bs=1024 count=512000 2>/dev/null
dd if=/dev/zero of="$DEMO_DIR/my-turborepo/apps/web/node_modules/.lock" bs=1 count=32 2>/dev/null
dd if=/dev/zero of="$DEMO_DIR/my-turborepo/apps/admin/.next/cache/data.json" bs=1024 count=384000 2>/dev/null
dd if=/dev/zero of="$DEMO_DIR/my-turborepo/packages/ui/node_modules/.lock" bs=1 count=16 2>/dev/null
echo '{}' > "$DEMO_DIR/my-turborepo/package.json"
echo '{}' > "$DEMO_DIR/my-turborepo/apps/web/package.json"
echo '{}' > "$DEMO_DIR/my-turborepo/apps/admin/package.json"
echo '{}' > "$DEMO_DIR/my-turborepo/packages/ui/package.json"
git -C "$DEMO_DIR/my-turborepo" init -q
git -C "$DEMO_DIR/my-turborepo" add -A
GIT_COMMITTER_DATE="2025-10-15T10:00:00" git -C "$DEMO_DIR/my-turborepo" -c user.email="demo@test.com" -c user.name="demo" commit -q -m "init" --date="2025-10-15T10:00:00"
touch -t 202510151000 "$DEMO_DIR/my-turborepo"

# 4. todo-api (small, active, with uncommitted changes — protected)
mkdir -p "$DEMO_DIR/todo-api/node_modules/express"
dd if=/dev/zero of="$DEMO_DIR/todo-api/node_modules/express/index.js" bs=1024 count=64000 2>/dev/null
echo '{}' > "$DEMO_DIR/todo-api/package.json"
git -C "$DEMO_DIR/todo-api" init -q
git -C "$DEMO_DIR/todo-api" add -A
git -C "$DEMO_DIR/todo-api" -c user.email="demo@test.com" -c user.name="demo" commit -q -m "init"
echo "// wip" >> "$DEMO_DIR/todo-api/package.json"
touch -t $(date -v-1d +%Y%m%d%H%M) "$DEMO_DIR/todo-api"

# --- Rust projects ---

# 5. cli-tool (medium, recent — 2 weeks ago)
mkdir -p "$DEMO_DIR/cli-tool/target/debug"
dd if=/dev/zero of="$DEMO_DIR/cli-tool/target/debug/cli-tool" bs=1024 count=768000 2>/dev/null
echo '[package]' > "$DEMO_DIR/cli-tool/Cargo.toml"
git -C "$DEMO_DIR/cli-tool" init -q
git -C "$DEMO_DIR/cli-tool" add -A
GIT_COMMITTER_DATE="2026-03-04T10:00:00" git -C "$DEMO_DIR/cli-tool" -c user.email="demo@test.com" -c user.name="demo" commit -q -m "init" --date="2026-03-04T10:00:00"
touch -t 202603041000 "$DEMO_DIR/cli-tool"
touch -t 202603041000 "$DEMO_DIR/cli-tool/target"

# 6. game-engine (large, dormant — 6 months ago)
mkdir -p "$DEMO_DIR/game-engine/target/release"
dd if=/dev/zero of="$DEMO_DIR/game-engine/target/release/engine" bs=1024 count=1536000 2>/dev/null
echo '[package]' > "$DEMO_DIR/game-engine/Cargo.toml"
git -C "$DEMO_DIR/game-engine" init -q
git -C "$DEMO_DIR/game-engine" add -A
GIT_COMMITTER_DATE="2025-09-15T10:00:00" git -C "$DEMO_DIR/game-engine" -c user.email="demo@test.com" -c user.name="demo" commit -q -m "init" --date="2025-09-15T10:00:00"
touch -t 202509151000 "$DEMO_DIR/game-engine"
touch -t 202509151000 "$DEMO_DIR/game-engine/target"

echo "Demo workspace created at $DEMO_DIR"
echo "Projects: 6 (4 Node.js, 2 Rust)"
