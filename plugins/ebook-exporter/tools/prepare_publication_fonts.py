"""Prepare local fonts for the approved Zhurongshuo publication design.

This optional build-time helper requires fontTools; the Go exporter itself
does not invoke Python. Fonts are read from the local OS, never downloaded.
Run again when the site's character set changes. Font files stay local.
"""
import argparse
from pathlib import Path
from fontTools.ttLib import TTFont
from fontTools import subset

p = argparse.ArgumentParser(description=__doc__)
p.add_argument("site", type=Path)
p.add_argument("--cjk-source", default="/System/Library/Fonts/STHeiti Light.ttc")
p.add_argument("--cjk-index", type=int, default=1)
p.add_argument("--latin-source", default="/System/Library/Fonts/Supplemental/Times New Roman.ttf")
args = p.parse_args()
text = "".join(chr(i) for i in range(32, 127))
text += "祝融说法不净空觉无性也版权所有保留一切权利版次更新日期年月日全书完著目录第一部卷引言结语章节附录版本电子书出版排版样张观察经验……•·©–—“”‘’（）：！"
for directory in (args.site / "content", args.site / "data"):
    for path in directory.rglob("*"):
        if path.suffix in (".md", ".yaml", ".yml"):
            text += path.read_text()
out = args.site / "developer/audit-tools/publication-fonts"
out.mkdir(parents=True, exist_ok=True)
for source, index, name in [(args.cjk_source, args.cjk_index, "body-cover-cjk.ttf"),
                             (args.latin_source, -1, "cover-latin.ttf")]:
    font = TTFont(source, fontNumber=index)
    if font["OS/2"].fsType & 2:
        raise ValueError(f"Font prohibits embedding: {source}")
    if "glyf" not in font:
        raise ValueError(f"PDF requires TrueType outlines: {source}")
    sub = subset.Subsetter()
    sub.populate(text=text)
    sub.subset(font)
    font.save(out / name)
    print(out / name)
