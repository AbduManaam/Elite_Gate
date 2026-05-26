from pathlib import Path
import sys, textwrap
from xml.sax.saxutils import escape

sys.path.insert(0, str(Path(__file__).parent))
import build_admin_auth_pdf as guide

from reportlab.lib import colors
from reportlab.lib.enums import TA_CENTER
from reportlab.lib.pagesizes import LETTER
from reportlab.lib.styles import getSampleStyleSheet, ParagraphStyle
from reportlab.lib.units import inch
from reportlab.platypus import SimpleDocTemplate, Paragraph, Spacer, Table, TableStyle, XPreformatted, PageBreak, KeepTogether

OUT = Path(r"C:\Users\abdum\OneDrive\Desktop\New folder\Coding\CoreGuard Gateway\docs\elitegate_admin_auth_complete_code_guide.pdf")

styles = getSampleStyleSheet()
styles.add(ParagraphStyle(
    name="GuideTitle", parent=styles["Title"], fontName="Helvetica-Bold", fontSize=21,
    leading=25, textColor=colors.HexColor("#0B2545"), spaceAfter=8,
))
styles.add(ParagraphStyle(
    name="GuideSubtitle", parent=styles["BodyText"], fontName="Helvetica-Oblique", fontSize=10.5,
    leading=14, textColor=colors.HexColor("#555555"), spaceAfter=14,
))
styles.add(ParagraphStyle(
    name="H1Guide", parent=styles["Heading1"], fontName="Helvetica-Bold", fontSize=14,
    leading=17, textColor=colors.HexColor("#2E74B5"), spaceBefore=14, spaceAfter=7,
))
styles.add(ParagraphStyle(
    name="BodyGuide", parent=styles["BodyText"], fontName="Helvetica", fontSize=9.5,
    leading=13, spaceAfter=6,
))
styles.add(ParagraphStyle(
    name="CodeLabel", parent=styles["BodyText"], fontName="Helvetica-Bold", fontSize=8.5,
    leading=10, textColor=colors.HexColor("#1F4D78"), spaceBefore=4, spaceAfter=3,
))
styles.add(ParagraphStyle(
    name="CodeBlockGuide", parent=styles["Code"], fontName="Courier", fontSize=6.8,
    leading=8.1, leftIndent=0, rightIndent=0, spaceAfter=7,
    backColor=colors.HexColor("#F6F8FA"), borderColor=colors.HexColor("#DADCE0"),
    borderWidth=0.25, borderPadding=4,
))

def wrap_code(code, width=112):
    out = []
    for line in code.strip("\n").split("\n"):
        if len(line) <= width:
            out.append(line)
        else:
            indent = len(line) - len(line.lstrip(" "))
            prefix = " " * min(indent + 4, 12)
            chunks = textwrap.wrap(line, width=width, subsequent_indent=prefix, break_long_words=False, break_on_hyphens=False)
            out.extend(chunks or [line])
    return "\n".join(out)

def footer(canvas, doc):
    canvas.saveState()
    canvas.setFont("Helvetica", 8)
    canvas.setFillColor(colors.HexColor("#666666"))
    canvas.drawString(inch, 0.5 * inch, "EliteGate Admin Auth Code Guide")
    canvas.drawRightString(LETTER[0] - inch, 0.5 * inch, f"Page {doc.page}")
    canvas.restoreState()

doc = SimpleDocTemplate(
    str(OUT), pagesize=LETTER, rightMargin=0.72*inch, leftMargin=0.72*inch,
    topMargin=0.72*inch, bottomMargin=0.72*inch,
)

story = []
story.append(Paragraph(escape(guide.DOC_TITLE), styles["GuideTitle"]))
story.append(Paragraph(escape(guide.SUBTITLE), styles["GuideSubtitle"]))
meta = Table([["Project", "EliteGate Gateway"], ["Preset", "compact_reference_guide"], ["Scope", "Admin auth implementation reference"]], colWidths=[1.45*inch, 5.25*inch])
meta.setStyle(TableStyle([
    ("GRID", (0,0), (-1,-1), 0.25, colors.HexColor("#C9D3DF")),
    ("BACKGROUND", (0,0), (0,-1), colors.HexColor("#E8EEF5")),
    ("FONT", (0,0), (0,-1), "Helvetica-Bold", 8.5),
    ("FONT", (1,0), (1,-1), "Helvetica", 8.5),
    ("VALIGN", (0,0), (-1,-1), "MIDDLE"),
    ("LEFTPADDING", (0,0), (-1,-1), 6),
    ("RIGHTPADDING", (0,0), (-1,-1), 6),
    ("TOPPADDING", (0,0), (-1,-1), 5),
    ("BOTTOMPADDING", (0,0), (-1,-1), 5),
]))
story.append(meta)
story.append(Spacer(1, 0.12*inch))

for idx, (title, body, code, lang) in enumerate(guide.sections, start=1):
    story.append(Paragraph(escape(f"{idx}. {title}"), styles["H1Guide"]))
    if body:
        for para in body.split("\n"):
            if para.strip():
                story.append(Paragraph(escape(para.strip()), styles["BodyGuide"]))
    if code:
        story.append(Paragraph(escape(lang or "code"), styles["CodeLabel"]))
        story.append(XPreformatted(escape(wrap_code(code)), styles["CodeBlockGuide"]))

story.append(PageBreak())
story.append(Paragraph("Final Implementation Notes", styles["H1Guide"]))
for note in [
    "Refresh token rotation is included and should be atomic: revoke the old token and insert the new token in one DB transaction.",
    "HTTP-layer login rate limiting is included separately from DB account lockout.",
    "JWT key rotation is intentionally deferred; use a required, strong 32+ byte JWT_SECRET and 15-minute access tokens for this phase.",
    "If browser cookie auth is added later, add httpOnly Secure SameSite cookies and CSRF protection then. The current API/Postman design returns JSON tokens.",
]:
    story.append(Paragraph(escape(note), styles["BodyGuide"]))

doc.build(story, onFirstPage=footer, onLaterPages=footer)
print(OUT)
