#!/usr/bin/env node
/**
 * Build styled PDF from HTTP-gRPC-Gateway-Setup-Guide.md
 * Run: node docs/build_http_grpc_gateway_pdf.mjs
 */
import { readFileSync, writeFileSync } from "fs";
import { dirname, join } from "path";
import { fileURLToPath, pathToFileURL } from "url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const MD_PATH = join(__dirname, "HTTP-gRPC-Gateway-Setup-Guide.md");
const CSS_PATH = join(__dirname, "pdf-style.css");
const HTML_PATH = join(__dirname, "_gateway_guide_build.html");
const OUT_PDF = join(__dirname, "EliteGuard-HTTP-gRPC-Gateway-Complete-Guide.pdf");

const REQUIREMENTS = [
  ["Sample HTTP services", "http-user (:9001), http-order (:9002)"],
  ["Sample gRPC service", "grpc-hello (:50052)"],
  ["Gateway in the middle", "Clients use :8080 (HTTP) and :50051 (gRPC)"],
  ["HTTP forwarding", "Dynamic reverse proxy per DB route"],
  ["gRPC forwarding", "Transparent gRPC proxy"],
  ["DB storing routes", "Postgres routes + upstreams, admin CRUD, gateway reload"],
];

function escapeHtml(s) {
  return String(s)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

function inline(s) {
  return s
    .replace(/\*\*(.+?)\*\*/g, "<strong>$1</strong>")
    .replace(/`([^`]+)`/g, "<code>$1</code>");
}

function mdToHtml(md) {
  let html = md.replace(/## 15\. Export to PDF[\s\S]*?(?=## 16\.)/, "");

  const codeBlocks = [];
  html = html.replace(/```(\w*)\n([\s\S]*?)```/g, (_, lang, code) => {
    const i = codeBlocks.length;
    codeBlocks.push({ lang: lang || "code", code });
    return `@@CODEBLOCK${i}@@`;
  });

  const tableLines = [];
  html = html.split("\n").map((line) => {
    if (line.trim().startsWith("|") && line.includes("|", 1)) {
      if (/^\|[\s\-:|]+\|$/.test(line.trim())) return null;
      const cells = line.trim().slice(1, -1).split("|").map((c) => c.trim());
      tableLines.push(cells);
      return null;
    }
    if (tableLines.length) {
      const rows = tableLines.splice(0, tableLines.length);
      const [head, ...body] = rows;
      const th = head.map((c) => `<th>${inline(c)}</th>`).join("");
      const trs = body
        .map((r) => `<tr>${r.map((c) => `<td>${inline(c)}</td>`).join("")}</tr>`)
        .join("");
      return `<table><thead><tr>${th}</tr></thead><tbody>${trs}</tbody></table>`;
    }
    return line;
  }).join("\n");

  html = html.replace(/^### (.+)$/gm, "<h3>$1</h3>");
  html = html.replace(/^## (.+)$/gm, "<h2>$1</h2>");
  html = html.replace(/^# (.+)$/gm, "");

  html = inline(html);
  html = html.replace(/^---$/gm, "<hr/>");

  html = html
    .split("\n")
    .map((line) => {
      const t = line.trim();
      if (!t) return "";
      if (/^<(h[23]|table|thead|tbody|tr|th|td|hr|pre|div|p|ol|ul|li|strong|code)/.test(t)) return line;
      if (t.startsWith("@@CODE")) return line;
      if (/^[┌│└▼→]/.test(t) || t.startsWith("Client →") || t.startsWith("grpcurl"))
        return `<pre class="diagram">${escapeHtml(t)}</pre>`;
      return `<p>${line}</p>`;
    })
    .join("\n");

  codeBlocks.forEach((b, i) => {
    const label = b.lang !== "code" ? `<div class="code-label">${escapeHtml(b.lang)}</div>` : "";
    html = html.replace(
      `@@CODEBLOCK${i}@@`,
      `${label}<pre><code>${escapeHtml(b.code)}</code></pre>`
    );
  });

  return html;
}

function buildCover() {
  const rows = REQUIREMENTS.map(
    ([a, b]) => `<tr><td><strong>${escapeHtml(a)}</strong></td><td>${escapeHtml(b)}</td></tr>`
  ).join("");
  return `
<div class="cover-banner">
  <h1>EliteGuard Gateway</h1>
  <p class="subtitle">HTTP &amp; gRPC Setup — Complete Implementation Guide</p>
  <p class="tagline">Sample services · Gateway in the middle · HTTP &amp; gRPC forwarding · DB route storage</p>
</div>
<h2>Scope covered by this guide</h2>
<table class="scope-table">
  <thead><tr><th>Requirement</th><th>What you get</th></tr></thead>
  <tbody>${rows}</tbody>
</table>
<table class="scope-table">
  <tr><td><strong>Project</strong></td><td>CoreGuard Gateway / EliteGuard (Go module: elitegate)</td></tr>
  <tr><td><strong>Version</strong></td><td>1.0 — May 2026</td></tr>
  <tr><td><strong>Phases</strong></td><td>16 implementation phases with full source code</td></tr>
</table>
`;
}

function buildToc(md) {
  const h2 = [...md.matchAll(/^## (\d+\.\s+.+)$/gm)].map((m) => m[1]);
  const filtered = h2.filter(
    (t) => !t.toLowerCase().includes("table of contents") && !t.startsWith("15.")
  );
  const items = filtered.map((t) => `<li>${escapeHtml(t)}</li>`).join("");
  return `<div class="toc"><h2>Table of Contents</h2><ol>${items}</ol>
<p class="footer-note">Recommended order: Phases 1–2 → 3 → 6 → 9 (HTTP demo) → 4–7 (gRPC) → 8 (Admin API)</p></div>`;
}

async function pdfWithPuppeteer(htmlPath, outPath) {
  const puppeteer = await import("puppeteer");
  const browser = await puppeteer.default.launch({ headless: true });
  const page = await browser.newPage();
  await page.goto(pathToFileURL(htmlPath).href, { waitUntil: "networkidle0" });
  await page.pdf({
    path: outPath,
    format: "A4",
    printBackground: true,
    margin: { top: "18mm", bottom: "20mm", left: "16mm", right: "16mm" },
  });
  await browser.close();
}

async function main() {
  const md = readFileSync(MD_PATH, "utf8");
  const css = readFileSync(CSS_PATH, "utf8");
  const body = buildCover() + buildToc(md) + mdToHtml(md);
  const fullHtml = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8"/>
<title>EliteGuard HTTP gRPC Gateway Guide</title>
<style>${css}</style>
</head>
<body>${body}
<div class="footer-note">EliteGuard / CoreGuard Gateway — Implementation Guide v1.0</div>
</body>
</html>`;

  writeFileSync(HTML_PATH, fullHtml, "utf8");
  console.log("HTML:", HTML_PATH);

  await pdfWithPuppeteer(HTML_PATH, OUT_PDF);
  console.log("PDF:", OUT_PDF);
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
