#!/usr/bin/env node
/**
 * beforeShellExecution: block git add/commit of secret-like paths.
 * Reads hook JSON from stdin; prints permission JSON to stdout.
 */
const { execSync } = require("child_process");
const fs = require("fs");

const DENY_BASENAMES = new Set([
  ".env",
  "kubeconfig",
  "credentials.json",
  "service-account.json",
]);

const DENY_EXACT = new Set([".env"]);

const DENY_EXTENSIONS = new Set([
  ".pem",
  ".key",
  ".p12",
  ".pfx",
  ".kubeconfig",
]);

function isDeniedPath(p) {
  if (!p) return false;
  const norm = p.replace(/\\/g, "/");
  const base = norm.split("/").pop() || "";

  if (base === ".env.example") return false;
  if (DENY_EXACT.has(base)) return true;
  if (DENY_BASENAMES.has(base)) return true;
  if (base.startsWith(".env.")) return true;
  if (base.endsWith(".kubeconfig")) return true;

  for (const ext of DENY_EXTENSIONS) {
    if (base.endsWith(ext)) return true;
  }

  // paths like secrets/foo or **/secrets/**
  if (/(^|\/)secrets(\/|$)/i.test(norm)) return true;

  return false;
}

function stagedFiles() {
  try {
    const out = execSync("git diff --cached --name-only --diff-filter=ACM", {
      encoding: "utf8",
      stdio: ["ignore", "pipe", "ignore"],
    });
    return out.split(/\r?\n/).map((s) => s.trim()).filter(Boolean);
  } catch {
    return [];
  }
}

function pathsFromAddCommand(command) {
  // rough parse: git add [options] <paths>
  const withoutGit = command.replace(/^\s*git\s+add\s+/i, "");
  const tokens = withoutGit.match(/(?:[^\s"']+|"[^"]*"|'[^']*')+/g) || [];
  const paths = [];
  for (const raw of tokens) {
    if (raw.startsWith("-")) continue;
    paths.push(raw.replace(/^['"]|['"]$/g, ""));
  }
  return paths;
}

function main() {
  let input = "";
  try {
    input = fs.readFileSync(0, "utf8");
  } catch {
    input = "";
  }

  let payload = {};
  try {
    payload = JSON.parse(input || "{}");
  } catch {
    // fail open on malformed stdin
    process.stdout.write(JSON.stringify({ permission: "allow" }));
    return;
  }

  const command = String(payload.command || "");
  const isCommit = /\bgit\s+commit\b/i.test(command);
  const isAdd = /\bgit\s+add\b/i.test(command);

  if (!isCommit && !isAdd) {
    process.stdout.write(JSON.stringify({ permission: "allow" }));
    return;
  }

  let candidates = [];
  if (isAdd) {
    candidates = pathsFromAddCommand(command);
    // `git add -A` / `.` — check staged after is hard; scan common danger via staged + literal
    if (candidates.length === 0 || candidates.some((p) => p === "." || p === "-A" || p === "--all")) {
      candidates = candidates.concat(stagedFiles());
      // also deny if command clearly targets .env
      if (/\.env\b/.test(command) && !/\.env\.example\b/.test(command)) {
        candidates.push(".env");
      }
    }
  }
  if (isCommit) {
    candidates = candidates.concat(stagedFiles());
  }

  const denied = [...new Set(candidates)].filter(isDeniedPath);
  if (denied.length > 0) {
    const msg =
      "Blocked secret-like path(s): " +
      denied.join(", ") +
      ". Use .env.example only; see docs/architecture/17-secrets-management.md";
    process.stdout.write(
      JSON.stringify({
        permission: "deny",
        user_message: msg,
        agent_message: msg,
      })
    );
    return;
  }

  process.stdout.write(JSON.stringify({ permission: "allow" }));
}

main();
