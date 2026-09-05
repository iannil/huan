package render

import (
	"archive/zip"
	"bytes"
	"fmt"
	"github.com/iannil/huan-plugin-ebook-exporter/content"
	"io"
	"os"
	"regexp"
	"strings"
)

func installDOCXCover(path string, book *content.BookEntry, lang content.Lang, fonts CoverFonts, hasMeta bool) error {
	png, err := publicationCover(book, lang, fonts)
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	reader, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, entry := range reader.File {
		stream, err := entry.Open()
		if err != nil {
			return err
		}
		b, err := io.ReadAll(stream)
		stream.Close()
		if err != nil {
			return err
		}
		s := string(b)
		switch entry.Name {
		case "word/document.xml":
			start := strings.Index(s, "<w:body>")
			if start < 0 {
				return fmt.Errorf("DOCX missing body")
			}
			start += len("<w:body>")
			count := 1
			if hasMeta {
				count = 2
			}
			paras := regexp.MustCompile(`(?s)<w:p(?:\s[^>]*)?>.*?</w:p>`).FindAllStringIndex(s[start:], count)
			if len(paras) != count {
				return fmt.Errorf("DOCX missing title paragraphs")
			}
			s = s[:start] + publicationCoverParagraph(pdfHeaderTitle(book, lang)) + s[start+paras[count-1][1]:]
		case "word/_rels/document.xml.rels":
			rel := `<Relationship Id="rIdPublicationCover" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/publication-cover.png"/><Relationship Id="rIdPublicationEmpty" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/footer" Target="publication-empty-footer.xml"/>`
			s = strings.Replace(s, "</Relationships>", rel+"</Relationships>", 1)
		case "[Content_Types].xml":
			add := `<Override PartName="/word/publication-empty-footer.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.footer+xml"/>`
			if !strings.Contains(s, `Extension="png"`) {
				add += `<Default Extension="png" ContentType="image/png"/>`
			}
			s = strings.Replace(s, "</Types>", add+"</Types>", 1)
		}
		dst, err := w.CreateHeader(&entry.FileHeader)
		if err != nil {
			return err
		}
		if _, err = dst.Write([]byte(s)); err != nil {
			return err
		}
	}
	for name, data := range map[string][]byte{"word/media/publication-cover.png": png, "word/publication-empty-footer.xml": []byte(`<w:ftr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:p/></w:ftr>`)} {
		dst, err := w.Create(name)
		if err != nil {
			return err
		}
		if _, err = dst.Write(data); err != nil {
			return err
		}
	}
	if err = w.Close(); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0644)
}
func publicationCoverParagraph(title string) string {
	title = escapeHTML(title)
	return `<w:p><w:pPr><w:spacing w:before="0" w:after="0" w:line="20" w:lineRule="exact"/><w:sectPr><w:footerReference w:type="default" r:id="rIdPublicationEmpty"/><w:type w:val="nextPage"/><w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="0" w:right="0" w:bottom="0" w:left="0" w:header="0" w:footer="0"/></w:sectPr></w:pPr><w:r><w:drawing><wp:anchor distT="0" distB="0" distL="0" distR="0" simplePos="0" relativeHeight="0" behindDoc="1" locked="0" layoutInCell="1" allowOverlap="1"><wp:simplePos x="0" y="0"/><wp:positionH relativeFrom="page"><wp:posOffset>0</wp:posOffset></wp:positionH><wp:positionV relativeFrom="page"><wp:posOffset>0</wp:posOffset></wp:positionV><wp:extent cx="7560310" cy="10692130"/><wp:effectExtent l="0" t="0" r="0" b="0"/><wp:wrapNone/><wp:docPr id="9101" name="Publication cover" descr="` + title + `"/><wp:cNvGraphicFramePr/><a:graphic xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:pic xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:nvPicPr><pic:cNvPr id="9101" name="publication-cover.png"/><pic:cNvPicPr/></pic:nvPicPr><pic:blipFill><a:blip r:embed="rIdPublicationCover"/><a:stretch><a:fillRect/></a:stretch></pic:blipFill><pic:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="7560310" cy="10692130"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr></pic:pic></a:graphicData></a:graphic></wp:anchor></w:drawing></w:r></w:p>`
}
