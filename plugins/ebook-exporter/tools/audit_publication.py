"""Audit exported publication artifacts without changing them.

Usage: python3 audit_publication.py EXPORT_ROOT --report REPORT_DIR
       [--epubcheck /path/to/epubcheck.jar] [--approved-docx sample.docx]
Requires PyMuPDF, Pillow and lxml. EPUBCheck requires Java.
"""
from pathlib import Path
import argparse, base64, collections, hashlib, io, json, re, subprocess, zipfile
from concurrent.futures import ThreadPoolExecutor
import fitz
from PIL import Image, ImageDraw
from lxml import etree

p = argparse.ArgumentParser(description=__doc__)
p.add_argument("root", type=Path)
p.add_argument("--report", type=Path, required=True)
p.add_argument("--epubcheck", type=Path)
p.add_argument("--approved-docx", type=Path)
args = p.parse_args()
args.report.mkdir(parents=True, exist_ok=True)
(args.report / "covers").mkdir(exist_ok=True)
results = []
styles = None
def style_signature(raw):
    # docxgo serializes its style map in nondeterministic order. Compare
    # definitions by style ID, excluding insignificant XML indentation.
    root = etree.fromstring(raw, etree.XMLParser(remove_blank_text=True))
    root[:] = sorted(root, key=lambda e: (e.tag, e.get("{http://schemas.openxmlformats.org/wordprocessingml/2006/main}styleId", "")))
    return etree.tostring(root, method="c14n")
if args.approved_docx:
    with zipfile.ZipFile(args.approved_docx) as z:
        styles = style_signature(z.read("word/styles.xml"))

def epub_check(path):
    r = subprocess.run(["java", "-jar", str(args.epubcheck), "--quiet", str(path)], capture_output=True, text=True)
    return {"file": str(path.relative_to(args.root)), "exit": r.returncode, "output": r.stdout + r.stderr}

pdfs = sorted((args.root / "pdf").rglob("*.pdf"))
for index, path in enumerate(pdfs):
    rel = path.relative_to(args.root / "pdf").with_suffix("")
    epub = args.root / "epub" / rel.with_suffix(".epub")
    docx = args.root / "docx" / rel.with_suffix(".docx")
    record = {"file": str(path.relative_to(args.root)), "errors": [], "warnings": []}
    def error(message): record["errors"].append(message)
    with zipfile.ZipFile(epub) as e, zipfile.ZipFile(docx) as w:
        if e.testzip() or w.testzip(): error("archive CRC failure")
        for z in (e, w):
            for name in z.namelist():
                if name.endswith((".xml", ".rels", ".xhtml", ".opf", ".svg")):
                    try: etree.fromstring(z.read(name))
                    except etree.XMLSyntaxError as exc: error(f"{name}: {exc}")
        svg = etree.fromstring(e.read("EPUB/images/cover.svg"))
        href = svg.find("{http://www.w3.org/2000/svg}image").get("{http://www.w3.org/1999/xlink}href")
        epub_cover = base64.b64decode(href.split(",", 1)[1])
        docx_cover = w.read("word/media/publication-cover.png")
        if epub_cover != docx_cover: error("EPUB/DOCX cover mismatch")
        if styles is not None and style_signature(w.read("word/styles.xml")) != styles: error("DOCX approved styles changed")
        cover = Image.open(io.BytesIO(docx_cover)).convert("RGB")
        record["cover_pixels"] = list(cover.size)
        for x,y,c in [(510,100,(173,47,46)), (510,343,(252,251,239)), (510,421,(252,251,239)), (510,499,(252,251,239)), (100,100,(255,255,255))]:
            if cover.getpixel((int(x/595.3*cover.width),int(y/841.9*cover.height))) != c:
                error(f"cover palette/mark mismatch at {x},{y}")
        cover.thumbnail((298,421))
        cover.save(args.report / "covers" / f"{index:03}.png")
    with fitz.open(path) as d:
        record["pages"] = len(d)
        record["bookmarks"] = len(d.get_toc())
        seen_fonts = set()
        leading = collections.Counter()
        overflow = []
        # Compare the actual PDF image pixels with the identical EPUB/DOCX cover.
        images = d[0].get_images()
        if not images: error("PDF cover missing")
        else:
            pdf_cover = Image.open(io.BytesIO(d.extract_image(images[0][0])["image"])).convert("RGB")
            if pdf_cover.tobytes() != Image.open(io.BytesIO(docx_cover)).convert("RGB").tobytes():
                error("PDF cover pixels differ from EPUB/DOCX")
        for n, page in enumerate(d):
            if abs(page.rect.width-595.3)>.1 or abs(page.rect.height-841.9)>.1: error(f"page {n+1}: non-A4")
            if n > 0:
                footer = page.get_text(clip=fitz.Rect(250,795,345,825)).strip()
                if footer != str(n+1): error(f"page {n+1}: footer {footer!r}")
            for f in page.get_fonts():
                if f[0] in seen_fonts: continue
                seen_fonts.add(f[0])
                raw = d.extract_font(f[0])[3]
                if raw and f[1] == "ttf" and raw[:4] not in (b"\0\1\0\0", b"true"):
                    error(f"font {f[3]} has invalid TrueType container")
            if n == 0: continue
            spans = [s for b in page.get_text("dict")["blocks"] for l in b.get("lines",[]) for s in l["spans"]]
            ys = sorted(set(round(s["origin"][1],2) for s in spans if abs(s["size"]-12)<.1))
            leading.update(round(b-a,1) for a,b in zip(ys,ys[1:]) if 10<b-a<35)
            for s in spans:
                x0,y0,x1,y1 = s["bbox"]
                if s["size"] >= 9.5 and (x0<50 or x1>page.rect.width-50 or y1>790):
                    overflow.append({"page":n+1,"text":s["text"][:100],"bbox":s["bbox"]})
        record["body_leading_pt"] = dict(leading)
        record["overflow_candidates"] = overflow
        if leading and leading.most_common(1)[0][0] != 22.2: error("dominant body leading is not 22.2 pt")
        if overflow: record["warnings"].append(f"{len(overflow)} content boundary candidates require review")
    results.append(record)
    print(index+1, len(pdfs), rel, "errors", len(record["errors"]), flush=True)
    (args.report/"files.json").write_text(json.dumps(results,ensure_ascii=False,indent=2))

checks=[]
if args.epubcheck:
    with ThreadPoolExecutor(max_workers=4) as pool:
        checks=list(pool.map(epub_check, sorted((args.root/"epub").rglob("*.epub"))))
    (args.report/"epubcheck.json").write_text(json.dumps(checks,ensure_ascii=False,indent=2))
for start in range(0,len(results),8):
    sheet=Image.new("RGB",(1200,900),"#dddddd")
    draw=ImageDraw.Draw(sheet)
    for offset,record in enumerate(results[start:start+8]):
        x=(offset%4)*300;y=(offset//4)*450
        sheet.paste(Image.open(args.report/"covers"/f"{start+offset:03}.png"),(x,y+23))
        draw.text((x+4,y+5),f'{start+offset:03} '+Path(record["file"]).stem[:36],fill="black")
    sheet.save(args.report/f"covers-{start:03}.jpg")
summary={"books":len(results),"pdf_pages":sum(r["pages"] for r in results),"errors":sum(len(r["errors"]) for r in results),"boundary_candidates":sum(len(r["overflow_candidates"]) for r in results),"epubcheck_failed":sum(r["exit"]!=0 for r in checks)}
(args.report/"summary.json").write_text(json.dumps(summary,indent=2))
print(summary)
if summary["errors"] or summary["epubcheck_failed"]: raise SystemExit(1)
