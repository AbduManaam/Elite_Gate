#!/usr/bin/env python3
"""
Build styled PDF from docs/HTTP-gRPC-Gateway-Setup-Guide.md
Run: python docs/build_http_grpc_gateway_pdf.py
"""
from __future__ import annotations

import re
import textwrap
from pathlib import Path
from xml.sax.saxutils import escape

from reportlab.lib import colors
from reportlab.lib.enums import TA_CENTER, TA_LEFT
from reportlab.lib.pagesizes import LETTER
from reportlab.lib.styles import ParagraphStyle, getSampleStyleSheet
from reportlab.lib.units import inch
from reportlab.platypus import (
    KeepTogether,
    PageBreak,
    Paragraph,
    SimpleDocTemplate,
    Spacer,
    Table,
    TableStyle,
    XPreformatted,
)

ROOT = Path(__file__).resolve().parent
MD_PATH = ROOT / "HTTP-gRPC-Gateway-Setup-Guide.md"
OUT_PATH = ROOT / "EliteGuard-HTTP-gRPC-Gateway-Complete-Guide.pdf"

# Brand colors
NAVY = colors.HexColor("#0B2545")
BLUE = colors.HexColor("#2E74B5")
LIGHT_BLUE = colors.HexColor("#E8EEF5")
CODE_BG = colors.HexColor("#F6F8FA")
CODE_BORDER = colors.HexColor("#DADCE0")
MUTED = colors.HexColor("#555555")
GREEN = colors.HexColor("#1B7F4B")
WHITE = colors.white

DOC_TITLE = "EliteGuard Gateway"
DOC_SUBTITLE = "HTTP & gRPC Setup — Complete Implementation Guide"
DOC_TAGLINE = (
    "Sample HTTP/gRPC services · API gateway in the middle · "
    "HTTP & gRPC forwarding · PostgreSQL route storage"
)

REQUIREMENTS = [
    ("Sample HTTP services", "http-user (:9001), http-order (:9002)"),
    ("Sample gRPC service", "grpc-hello (:50052)"),
    ("Gateway in the middle", "Clients use :8080 (HTTP) and :50051 (gRPC)"),
    ("HTTP forwarding", "Dynamic reverse proxy per DB route"),
    ("gRPC forwarding", "Transparent gRPC proxy"),
    ("DB storing routes", "Postgres routes + upstreams, admin CRUD, gateway reload"),
]


def wrap_code(code: str, width: int = 108) -> str:
    out: list[str] = []
    for line in code.strip("\n").split("\n"):
        if len(line) <= width:
            out.append(line)
        else:
            indent = len(line) - len(line.lstrip(" "))
            prefix = " " * min(indent + 4, 14)
            chunks = textwrap.wrap(
                line,
                width=width,
                subsequent_indent=prefix,
                break_long_words=False,
                break_on_hyphens=False,
            )
            out.extend(chunks or [line])
    return "\n".join(out)


def parse_markdown(text: str) -> list[dict]:
    """Parse guide into blocks: h1, h2, h3, p, code, table, hr."""
    blocks: list[dict] = []
    lines = text.split("\n")
    i = 0
    in_code = False
    code_lang = ""
    code_buf: list[str] = []
    table_buf: list[str] = []
    para_buf: list[str] = []

    def flush_para():
        nonlocal para_buf
        if para_buf:
            body = "\n".join(para_buf).strip()
            if body and not body.startswith("---"):
                blocks.append({"type": "p", "text": body})
            para_buf = []

    def flush_table():
        nonlocal table_buf
        if len(table_buf) >= 2:
            rows = []
            for row in table_buf:
                if re.match(r"^\|[-:\s|]+\|$", row.strip()):
                    continue
                cells = [c.strip() for c in row.strip().strip("|").split("|")]
                if cells:
                    rows.append(cells)
            if rows:
                blocks.append({"type": "table", "rows": rows})
        table_buf = []

    while i < len(lines):
        line = lines[i]

        if line.strip().startswith("```"):
            if in_code:
                blocks.append({"type": "code", "lang": code_lang, "text": "\n".join(code_buf)})
                code_buf = []
                code_lang = ""
                in_code = False
            else:
                flush_para()
                flush_table()
                in_code = True
                code_lang = line.strip("`").strip() or "code"
            i += 1
            continue

        if in_code:
            code_buf.append(line)
            i += 1
            continue

        if line.strip().startswith("|") and "|" in line[1:]:
            flush_para()
            table_buf.append(line)
            i += 1
            continue
        else:
            flush_table()

        if line.startswith("# ") and not line.startswith("## "):
            flush_para()
            blocks.append({"type": "h1", "text": line[2:].strip()})
            i += 1
            continue
        if line.startswith("## "):
            flush_para()
            title = line[3:].strip()
            # Skip meta TOC anchor-only duplicates
            if title.lower().startswith("table of contents"):
                i += 1
                continue
            if title == "15. Export to PDF":
                # Skip PDF export section — user already has this PDF
                while i < len(lines) and not (lines[i].startswith("## ") and "16." in lines[i]):
                    i += 1
                continue
            blocks.append({"type": "h2", "text": title})
            i += 1
            continue
        if line.startswith("### "):
            flush_para()
            blocks.append({"type": "h3", "text": line[4:].strip()})
            i += 1
            continue

        if line.strip() == "---":
            flush_para()
            i += 1
            continue

        if not line.strip():
            flush_para()
            i += 1
            continue

        para_buf.append(line)
        i += 1

    flush_para()
    flush_table()
    if in_code and code_buf:
        blocks.append({"type": "code", "lang": code_lang, "text": "\n".join(code_buf)})
    return blocks


def md_inline_to_xml(s: str) -> str:
    s = escape(s)
    s = re.sub(r"\*\*(.+?)\*\*", r"<b>\1</b>", s)
    s = re.sub(r"`([^`]+)`", r'<font face="Courier" size="8">\1</font>', s)
    return s


def build_styles():
    base = getSampleStyleSheet()
    base.add(ParagraphStyle(
        name="CoverTitle", fontName="Helvetica-Bold", fontSize=26,
        leading=30, textColor=WHITE, alignment=TA_CENTER, spaceAfter=10,
    ))
    base.add(ParagraphStyle(
        name="CoverSub", fontName="Helvetica", fontSize=12,
        leading=16, textColor=colors.HexColor("#B8D4F0"), alignment=TA_CENTER, spaceAfter=8,
    ))
    base.add(ParagraphStyle(
        name="CoverTag", fontName="Helvetica-Oblique", fontSize=10,
        leading=14, textColor=colors.HexColor("#D0E4F5"), alignment=TA_CENTER,
    ))
    base.add(ParagraphStyle(
        name="H2Guide", fontName="Helvetica-Bold", fontSize=13,
        leading=16, textColor=BLUE, spaceBefore=12, spaceAfter=6,
    ))
    base.add(ParagraphStyle(
        name="H3Guide", fontName="Helvetica-Bold", fontSize=10.5,
        leading=13, textColor=NAVY, spaceBefore=8, spaceAfter=4,
    ))
    base.add(ParagraphStyle(
        name="BodyGuide", fontName="Helvetica", fontSize=9.5,
        leading=13, textColor=colors.black, spaceAfter=5,
    ))
    base.add(ParagraphStyle(
        name="CodeLabel", fontName="Helvetica-Bold", fontSize=8,
        leading=10, textColor=colors.HexColor("#1F4D78"), spaceBefore=3, spaceAfter=2,
    ))
    base.add(ParagraphStyle(
        name="CodeBlock", fontName="Courier", fontSize=6.4,
        leading=7.6, backColor=CODE_BG, borderColor=CODE_BORDER,
        borderWidth=0.25, borderPadding=5, spaceAfter=6,
    ))
    base.add(ParagraphStyle(
        name="TOCItem", fontName="Helvetica", fontSize=9.5,
        leading=13, leftIndent=12, spaceAfter=3,
    ))
    base.add(ParagraphStyle(
        name="FooterNote", fontName="Helvetica-Oblique", fontSize=8,
        leading=10, textColor=MUTED,
    ))
    return base


def cover_page(story, styles):
    cover_data = [[Paragraph(f'<para align="center"><font color="#FFFFFF"><b>{escape(DOC_TITLE)}</b></font></para>', styles["BodyGuide"])]]
    # Use table as colored banner
    banner = Table(
        [[Paragraph(f'<para align="center"><font color="white" size="22"><b>{escape(DOC_TITLE)}</b></font></para>', styles["BodyGuide"])]],
        colWidths=[6.5 * inch],
        rowHeights=[1.1 * inch],
    )
    banner.setStyle(TableStyle([
        ("BACKGROUND", (0, 0), (-1, -1), NAVY),
        ("ALIGN", (0, 0), (-1, -1), "CENTER"),
        ("VALIGN", (0, 0), (-1, -1), "MIDDLE"),
    ]))
    story.append(Spacer(1, 0.35 * inch))
    story.append(banner)
    story.append(Spacer(1, 0.25 * inch))
    story.append(Paragraph(escape(DOC_SUBTITLE), ParagraphStyle(
        name="Sub", fontName="Helvetica-Bold", fontSize=14, leading=18,
        textColor=NAVY, alignment=TA_CENTER, spaceAfter=10,
    )))
    story.append(Paragraph(escape(DOC_TAGLINE), ParagraphStyle(
        name="Tag", fontName="Helvetica", fontSize=10, leading=14,
        textColor=MUTED, alignment=TA_CENTER, spaceAfter=20,
    )))

    req_rows = [[Paragraph("<b>Requirement</b>", styles["BodyGuide"]),
                 Paragraph("<b>What you get</b>", styles["BodyGuide"])]]
    for req, detail in REQUIREMENTS:
        req_rows.append([
            Paragraph(escape(req), styles["BodyGuide"]),
            Paragraph(escape(detail), styles["BodyGuide"]),
        ])
    req_table = Table(req_rows, colWidths=[2.0 * inch, 4.5 * inch])
    req_table.setStyle(TableStyle([
        ("BACKGROUND", (0, 0), (-1, 0), BLUE),
        ("TEXTCOLOR", (0, 0), (-1, 0), WHITE),
        ("FONT", (0, 0), (-1, 0), "Helvetica-Bold", 9),
        ("GRID", (0, 0), (-1, -1), 0.25, CODE_BORDER),
        ("ROWBACKGROUNDS", (0, 1), (-1, -1), [WHITE, LIGHT_BLUE]),
        ("VALIGN", (0, 0), (-1, -1), "TOP"),
        ("LEFTPADDING", (0, 0), (-1, -1), 8),
        ("RIGHTPADDING", (0, 0), (-1, -1), 8),
        ("TOPPADDING", (0, 0), (-1, -1), 6),
        ("BOTTOMPADDING", (0, 0), (-1, -1), 6),
    ]))
    story.append(Paragraph("<b>Scope covered by this guide</b>", styles["H3Guide"]))
    story.append(Spacer(1, 0.08 * inch))
    story.append(req_table)
    story.append(Spacer(1, 0.2 * inch))

    meta = Table([
        ["Project", "CoreGuard Gateway / EliteGuard (Go module: elitegate)"],
        ["Version", "1.0 — May 2026"],
        ["Source doc", "docs/HTTP-gRPC-Gateway-Setup-Guide.md"],
        ["Phases", "16 implementation phases with full code"],
    ], colWidths=[1.35 * inch, 5.15 * inch])
    meta.setStyle(TableStyle([
        ("GRID", (0, 0), (-1, -1), 0.25, CODE_BORDER),
        ("BACKGROUND", (0, 0), (0, -1), LIGHT_BLUE),
        ("FONT", (0, 0), (0, -1), "Helvetica-Bold", 8.5),
        ("FONT", (1, 0), (1, -1), "Helvetica", 8.5),
        ("VALIGN", (0, 0), (-1, -1), "MIDDLE"),
        ("LEFTPADDING", (0, 0), (-1, -1), 6),
        ("TOPPADDING", (0, 0), (-1, -1), 5),
        ("BOTTOMPADDING", (0, 0), (-1, -1), 5),
    ]))
    story.append(meta)
    story.append(PageBreak())


def toc_page(story, styles, h2_titles: list[str]):
    story.append(Paragraph("Table of Contents", styles["H2Guide"]))
    story.append(Spacer(1, 0.1 * inch))
    for i, title in enumerate(h2_titles, 1):
        clean = re.sub(r"^\d+\.\s*", "", title)
        story.append(Paragraph(f"{i}. {escape(clean)}", styles["TOCItem"]))
    story.append(Spacer(1, 0.15 * inch))
    story.append(Paragraph(
        "<i>Recommended build order: Phases 1–2 → 3 → 6 → 9 (HTTP demo) → 4–7 (gRPC) → 8 (Admin API)</i>",
        styles["FooterNote"],
    ))
    story.append(PageBreak())


def render_table(rows: list[list[str]], styles) -> Table:
    data = []
    for ri, row in enumerate(rows):
        data.append([Paragraph(md_inline_to_xml(c), styles["BodyGuide"]) for c in row])
    col_count = max(len(r) for r in rows)
    width = 6.5 * inch / col_count
    t = Table(data, colWidths=[width] * col_count, repeatRows=1)
    style_cmds = [
        ("GRID", (0, 0), (-1, -1), 0.25, CODE_BORDER),
        ("VALIGN", (0, 0), (-1, -1), "TOP"),
        ("LEFTPADDING", (0, 0), (-1, -1), 5),
        ("RIGHTPADDING", (0, 0), (-1, -1), 5),
        ("TOPPADDING", (0, 0), (-1, -1), 4),
        ("BOTTOMPADDING", (0, 0), (-1, -1), 4),
        ("FONT", (0, 0), (-1, 0), "Helvetica-Bold", 8.5),
        ("BACKGROUND", (0, 0), (-1, 0), BLUE),
        ("TEXTCOLOR", (0, 0), (-1, 0), WHITE),
    ]
    if len(rows) > 1:
        style_cmds.append(("ROWBACKGROUNDS", (0, 1), (-1, -1), [WHITE, LIGHT_BLUE]))
    t.setStyle(TableStyle(style_cmds))
    return t


def draw_header_footer(canvas, doc):
    canvas.saveState()
    canvas.setFont("Helvetica", 8)
    canvas.setFillColor(MUTED)
    canvas.drawString(inch, LETTER[1] - 0.45 * inch, "EliteGuard — HTTP & gRPC Gateway Guide")
    canvas.drawRightString(LETTER[0] - inch, 0.5 * inch, f"Page {doc.page}")
    canvas.setStrokeColor(CODE_BORDER)
    canvas.setLineWidth(0.5)
    canvas.line(inch, LETTER[1] - 0.5 * inch, LETTER[0] - inch, LETTER[1] - 0.5 * inch)
    canvas.restoreState()


def main():
    if not MD_PATH.exists():
        raise SystemExit(f"Missing markdown guide: {MD_PATH}")

    md_text = MD_PATH.read_text(encoding="utf-8")
    blocks = parse_markdown(md_text)
    styles = build_styles()

    h2_titles = [b["text"] for b in blocks if b["type"] == "h2"]

    doc = SimpleDocTemplate(
        str(OUT_PATH),
        pagesize=LETTER,
        rightMargin=0.65 * inch,
        leftMargin=0.65 * inch,
        topMargin=0.75 * inch,
        bottomMargin=0.65 * inch,
        title=DOC_TITLE,
        author="EliteGuard",
        subject="HTTP gRPC Gateway Setup",
    )

    story: list = []
    cover_page(story, styles)
    toc_page(story, styles, h2_titles)

    for block in blocks:
        t = block["type"]
        if t == "h1":
            continue  # skip duplicate doc title
        if t == "h2":
            story.append(Paragraph(md_inline_to_xml(block["text"]), styles["H2Guide"]))
        elif t == "h3":
            story.append(Paragraph(md_inline_to_xml(block["text"]), styles["H3Guide"]))
        elif t == "p":
            for para in block["text"].split("\n"):
                p = para.strip()
                if not p or p.startswith("```"):
                    continue
                # ASCII diagrams — monospace preformatted
                if p.startswith("┌") or p.startswith("│") or p.startswith("└") or p.startswith("▼") or p.startswith("→"):
                    story.append(XPreformatted(escape(p), styles["CodeBlock"]))
                elif p.startswith("Client →") or p.startswith("grpcurl"):
                    story.append(XPreformatted(escape(p), styles["CodeBlock"]))
                else:
                    story.append(Paragraph(md_inline_to_xml(p), styles["BodyGuide"]))
        elif t == "table":
            story.append(Spacer(1, 0.05 * inch))
            story.append(render_table(block["rows"], styles))
            story.append(Spacer(1, 0.08 * inch))
        elif t == "code":
            lang = block.get("lang", "code")
            if lang in ("mermaid",):
                continue
            label = lang if lang != "code" else "source"
            story.append(Paragraph(escape(f"📄 {label}"), styles["CodeLabel"]))
            story.append(XPreformatted(escape(wrap_code(block["text"])), styles["CodeBlock"]))

    story.append(PageBreak())
    story.append(Paragraph("Quick Start Commands", styles["H2Guide"]))
    quick = """
# Infrastructure
make infra-up
make docker-up
make seed-routes

# JWT for gateway
make token CLIENT=demo-client

# Test HTTP via gateway
curl -H "Authorization: Bearer <TOKEN>" http://localhost:8080/api/users/42

# Test gRPC via gateway
grpcurl -plaintext -d '{"name":"EliteGuard"}' localhost:50051 helloworld.v1.Greeter/SayHello
""".strip()
    story.append(XPreformatted(escape(wrap_code(quick)), styles["CodeBlock"]))
    story.append(Spacer(1, 0.15 * inch))
    story.append(Paragraph(
        "<b>End state:</b> Sample HTTP + gRPC backends, gateway in the middle, "
        "HTTP and gRPC forwarding, routes stored in PostgreSQL and managed via Admin API.",
        styles["BodyGuide"],
    ))

    doc.build(story, onFirstPage=draw_header_footer, onLaterPages=draw_header_footer)
    print(f"PDF written: {OUT_PATH}")
    print(f"Size: {OUT_PATH.stat().st_size / 1024:.1f} KB")


if __name__ == "__main__":
    main()
