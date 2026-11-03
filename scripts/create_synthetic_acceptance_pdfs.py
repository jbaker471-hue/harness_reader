from pathlib import Path
from reportlab.pdfgen import canvas
from reportlab.lib.pagesizes import landscape, letter
from reportlab.lib.colors import HexColor


ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / "testdata" / "synthetic_acceptance"
OUT.mkdir(parents=True, exist_ok=True)
PAGE = landscape(letter)


def title(c, name, sheet, total=2):
    c.setFillColor(HexColor("#173352"))
    c.rect(0, 575, 792, 37, fill=1, stroke=0)
    c.setFillColorRGB(1, 1, 1)
    c.setFont("Helvetica-Bold", 12)
    c.drawString(28, 592, f"NON-CUI SYNTHETIC HARNESS - {name}")
    c.setFont("Helvetica", 8)
    c.drawRightString(764, 592, f"SHEET {sheet} OF {total}  |  TEST DATA ONLY")
    c.setFillColorRGB(0, 0, 0)


def table(c, x, y, widths, headers, rows, row_h=17, font=7):
    total = sum(widths)
    c.setFillColor(HexColor("#E8EFF5"))
    c.rect(x, y-row_h, total, row_h, fill=1, stroke=1)
    c.setFillColorRGB(0, 0, 0)
    cx = x
    c.setFont("Helvetica-Bold", font)
    for width, value in zip(widths, headers):
        c.drawString(cx+3, y-row_h+5, value)
        cx += width
    c.setFont("Helvetica", font)
    for r, row in enumerate(rows):
        top = y-(r+1)*row_h
        c.rect(x, top-row_h, total, row_h, fill=0, stroke=1)
        cx = x
        for width, value in zip(widths, row):
            c.drawString(cx+3, top-row_h+5, str(value)[:45])
            cx += width
    cx = x
    for width in widths[:-1]:
        cx += width
        c.line(cx, y-row_h*(len(rows)+1), cx, y)


def make_pdf(filename, name, connectors, wires, bom, notes, callouts=()):
    path = OUT / filename
    c = canvas.Canvas(str(path), pagesize=PAGE, pageCompression=0)
    c.setTitle(f"NON-CUI SYNTHETIC {name}")
    title(c, name, 1)
    c.setFont("Helvetica-Bold", 10)
    c.drawString(30, 548, "CONNECTOR TABLE")
    table(c, 30, 536, [70, 180, 225], ["REF DES", "PART NUMBER", "DESCRIPTION"], connectors)
    c.setFont("Helvetica-Bold", 10)
    c.drawString(30, 430, "WIRE LIST")
    table(c, 30, 418, [52, 54, 32, 54, 32, 92, 38, 58, 60],
          ["WIRE NO.", "FROM", "PIN", "TO", "PIN", "SIGNAL NAME", "GAGE", "COLOR", "LENGTH"], wires)
    c.setFont("Helvetica-Bold", 9)
    c.drawString(575, 548, "SCHEMATIC LABELS")
    c.setFont("Helvetica", 8)
    for i, value in enumerate(callouts):
        c.drawString(575, 530-i*16, value)
    c.setStrokeColor(HexColor("#29A7A1"))
    c.setLineWidth(2)
    c.line(575, 470, 720, 470)
    c.circle(575, 470, 4, stroke=1, fill=0)
    c.circle(720, 470, 4, stroke=1, fill=0)
    c.showPage()

    title(c, name, 2)
    c.setFont("Helvetica-Bold", 10)
    c.drawString(30, 548, "BILL OF MATERIALS")
    table(c, 30, 536, [38, 42, 190, 260, 72],
          ["ITEM", "QTY", "PART NUMBER", "DESCRIPTION", "REF DES"], bom)
    c.setFont("Helvetica-Bold", 10)
    c.drawString(30, 330, "DRAWING NOTES")
    c.setFont("Helvetica", 8)
    y = 310
    for i, note in enumerate(notes, 1):
        c.drawString(38, y, f"{i}. {note}")
        y -= 22
    c.setFont("Helvetica-Bold", 8)
    c.drawString(30, 45, "SYNTHETIC ACCEPTANCE FIXTURE - NOT A PRODUCTION DRAWING - NO CUSTOMER DATA")
    c.save()
    return path


BASE_CONNECTORS = [
    ("P1", "D38999/26FJ35PA", "CONNECTOR PLUG - PIN CONTACTS"),
    ("J2", "D38999/26FJ35SA", "CONNECTOR PLUG - SOCKET CONTACTS"),
]
BASE_WIRES = [
    ("W1", "P1", "1", "J2", "A", "+28V", "22", "RED", "36 IN"),
    ("W2", "P1", "2", "J2", "B", "RETURN", "22", "BLK", "36 IN"),
    ("W3", "P1", "3", "J2", "C", "DATA_H", "24", "WHT/GRN", "40 IN"),
    ("W4", "P1", "4", "J2", "D", "DATA_L", "24", "WHT/BLU", "40 IN"),
]
BASE_BOM = [
    ("1", "1", "D38999/26FJ35PA", "CONNECTOR PLUG PIN", "P1"),
    ("2", "1", "D38999/26FJ35SA", "CONNECTOR PLUG SOCKET", "J2"),
    ("3", "4", "M39029/58-360", "CONTACT PIN 22D", "P1"),
    ("4", "4", "M39029/56-348", "CONTACT SOCKET 22D", "J2"),
]


paths = []
paths.append(make_pdf(
    "01_clean_two_connector.pdf", "CLEAN TWO CONNECTOR",
    BASE_CONNECTORS, BASE_WIRES, BASE_BOM,
    ["ASSEMBLE PER IPC/WHMA-A-620. VERIFY CONTINUITY 100 PERCENT."]
))

paths.append(make_pdf(
    "02_banded_backshell_shield_bond.pdf", "BANDED BACKSHELL SHIELD BOND",
    BASE_CONNECTORS,
    [("C1-1", "P1", "5", "J2", "E", "CAN_H", "24", "WHT/GRN", "48 IN"),
     ("C1-2", "P1", "6", "J2", "F", "CAN_L", "24", "WHT/BLU", "48 IN")],
    BASE_BOM + [("5", "1", "M85049/128-1", "BACKSHELL EMI SHIELD BAND TYPE", "P1"),
                ("6", "1", "M85049/128-2", "BACKSHELL EMI SHIELD BAND TYPE", "J2")],
    ["CABLE C1 IS A TWISTED SHIELDED PAIR.",
     "TERMINATE C1 SHIELD 360 DEG TO THE BACKSHELL AT P1 AND J2 USING A SHIELD BAND.",
     "USE THE MANUFACTURER SPECIFIED BAND INSTALLATION TOOL OR A DOCUMENTED QUALIFIED ALTERNATE."],
    ["C1 SHIELD TO P1 BACKSHELL", "C1 SHIELD TO J2 BACKSHELL"]
))

paths.append(make_pdf(
    "03_float_and_solder_sleeve.pdf", "FLOAT AND SOLDER SLEEVE",
    BASE_CONNECTORS,
    [("C2-1", "P1", "7", "J2", "G", "RS422_P", "24", "WHT/ORG", "30 IN"),
     ("C2-2", "P1", "8", "J2", "H", "RS422_N", "24", "WHT/BRN", "30 IN")],
    BASE_BOM + [("5", "1", "M83519/1-2", "SOLDER SLEEVE SHIELD TERMINATION", "J2")],
    ["CABLE C2 IS A TWISTED SHIELDED PAIR.",
     "AT P1 CUT BACK, INSULATE, AND FLOAT THE C2 SHIELD.",
     "AT J2 TERMINATE THE C2 SHIELD TO CONTACT 9 USING A SOLDER SLEEVE."],
    ["P1 END: SHIELD FLOAT", "J2 END: SHIELD TO PIN 9"]
))

paths.append(make_pdf(
    "04_clamp_and_crimp_ring.pdf", "CLAMP AND CRIMP RING BACKSHELLS",
    BASE_CONNECTORS, BASE_WIRES,
    BASE_BOM + [("5", "1", "M85049/38S15W", "CLAMP STYLE STRAIN RELIEF BACKSHELL", "P1"),
                ("6", "1", "SYN-CR-15", "BACKSHELL CRIMP RING", "J2")],
    ["ASSEMBLE THE P1 CLAMP BACKSHELL WITH THE SPECIFIED SMALL FLAT BLADE DRIVER.",
     "CRIMP THE J2 BACKSHELL RING WITH THE MANUFACTURER SPECIFIED CRIMP RING TOOL.",
     "ALTERNATE TOOLING IS ALLOWED ONLY WHEN DOCUMENTED AS EQUIVALENT FOR THE PART AND PROCESS."],
    ["P1 CLAMP BACKSHELL", "J2 CRIMP RING BACKSHELL"]
))

paths.append(make_pdf(
    "05_cots_branch_endpoint.pdf", "COTS CABLE AND BRANCH ENDPOINT",
    BASE_CONNECTORS,
    [("COTS1", "P1", "10", "J2", "J", "ETHERNET", "", "", "72 IN")],
    BASE_BOM + [("5", "1", "SYN-COTS-CBL-001", "COTS PURCHASED CABLE ASSEMBLY", "COTS1")],
    ["COTS1 IS A PURCHASED COMMERCIAL CABLE ASSEMBLY.",
     "CB-P3 IS A CABLE BREAKOUT BRANCH LABEL ONLY AND IS NOT A CONNECTOR."],
    ["CB-P3", "COTS1 PURCHASED ASSEMBLY"]
))

paths.append(make_pdf(
    "06_conflict_and_missing_data.pdf", "CONFLICT AND MISSING DATA",
    BASE_CONNECTORS,
    [("C3-1", "P1", "11", "J2", "K", "DATA_P", "24", "WHT", ""),
     ("C3-2", "P1", "12", "J2", "L", "DATA_N", "24", "BLU", "")],
    [],
    ["CABLE C3 IS A TWISTED SHIELDED PAIR.",
     "AT P1 FLOAT THE C3 SHIELD.",
     "AT P1 TERMINATE THE C3 SHIELD TO THE BACKSHELL.",
     "C3 LENGTH IS NOT STATED. BOM IS NOT PROVIDED."],
    ["CONFLICT: P1 SHIELD FLOAT / BACKSHELL", "LENGTH NOT STATED"]
))

print("\n".join(str(p) for p in paths))
