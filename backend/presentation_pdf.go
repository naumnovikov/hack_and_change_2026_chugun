package main

import (
	"bytes"
	_ "embed"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"

	"github.com/go-pdf/fpdf"
	"golang.org/x/image/font/gofont/goregular"
)

// Include distribution notices in the executable as well as in the source/image.
//
//go:embed GO_FONT_LICENSE.txt
var goFontLicense string

//go:embed FPDF_LICENSE.txt
var fpdfLicense string

//go:embed X_IMAGE_LICENSE.txt
var xImageLicense string

// fpdf indexes UTF-8 widths through a 65536-entry table. Check the embedded
// font's BMP cmap before any fpdf call, both to prevent non-BMP panics and to
// reject missing glyphs instead of silently losing source characters.
var pdfFontGlyphs = embeddedPDFGlyphs()

func embeddedPDFGlyphs() [65536]bool {
	var supported [65536]bool
	font := goregular.TTF
	u16 := func(offset int) int { return int(binary.BigEndian.Uint16(font[offset:])) }
	u32 := func(offset int) int { return int(binary.BigEndian.Uint32(font[offset:])) }
	for table := 0; table < u16(4); table++ {
		entry := 12 + 16*table
		if string(font[entry:entry+4]) != "cmap" {
			continue
		}
		cmap := u32(entry + 8)
		for record := 0; record < u16(cmap+2); record++ {
			entry := cmap + 4 + 8*record
			platform, encoding := u16(entry), u16(entry+2)
			if platform != 0 && !(platform == 3 && encoding == 1) {
				continue
			}
			sub := cmap + u32(entry+4)
			if u16(sub) != 4 {
				continue
			}
			count := u16(sub+6) / 2
			ends := sub + 14
			starts := ends + 2*count + 2
			deltas := starts + 2*count
			ranges := deltas + 2*count
			for segment := 0; segment < count; segment++ {
				for code := u16(starts + 2*segment); code <= u16(ends+2*segment) && code < 65535; code++ {
					glyph := 0
					if distance := u16(ranges + 2*segment); distance == 0 {
						glyph = (code + u16(deltas+2*segment)) & 65535
					} else {
						glyph = u16(ranges + 2*segment + distance + 2*(code-u16(starts+2*segment)))
						if glyph != 0 {
							glyph = (glyph + u16(deltas+2*segment)) & 65535
						}
					}
					if glyph != 0 {
						supported[code] = true
					}
				}
			}
		}
	}
	supported['\n'] = true
	return supported
}

func validatePDFText(d presentationDraft, tasks []presentationTask) error {
	texts := []string{}
	for _, task := range tasks {
		texts = append(texts, task.Key, task.Summary, task.Description)
	}
	for _, slide := range d.Slides {
		texts = append(texts, slide.Title)
		for _, bullet := range slide.Bullets {
			texts = append(texts, bullet.Text)
			texts = append(texts, bullet.SourceKeys...)
		}
	}
	for _, text := range texts {
		for _, r := range text {
			if r > 65535 || !pdfFontGlyphs[r] {
				return &presentationError{422, "unsupported_pdf_text"}
			}
		}
	}
	return nil
}

func renderPresentation(d presentationDraft, tasks []presentationTask) ([]byte, error) {
	if err := validatePresentation(d, tasks); err != nil {
		return nil, err
	}
	if err := validatePDFText(d, tasks); err != nil {
		return nil, err
	}
	pdf := fpdf.NewCustom(&fpdf.InitType{UnitStr: "mm", Size: fpdf.SizeType{Wd: 297, Ht: 167}})
	pdf.SetAutoPageBreak(false, 0)
	pdf.SetCompression(false)
	pdf.AddUTF8FontFromBytes("GoRegular", "", goregular.TTF)
	pdf.SetTitle("Мои результаты — Демо-черновик", true)
	pdf.SetAuthor("Команда Чугун", true)
	pdf.SetSubject("Синтетические Jira-задачи. Go Regular: Copyright (c) 2016 Bigelow & Holmes Inc.", true)
	pdf.SetKeywords(goFontLicense+"\n"+fpdfLicense+"\n"+xImageLicense, true)
	for i, s := range d.Slides {
		pdf.AddPage()
		pdf.SetFillColor(246, 247, 249)
		pdf.Rect(0, 0, 297, 167, "F")
		for n, c := range [][3]int{{0, 170, 255}, {151, 207, 38}, {163, 98, 239}, {255, 97, 99}} {
			pdf.SetFillColor(c[0], c[1], c[2])
			pdf.Circle(18+float64(n)*6, 12, 2, "F")
		}
		pdf.SetTextColor(25, 25, 30)
		pdf.SetFont("GoRegular", "", 20)
		pdf.SetXY(15, 21)
		pdf.CellFormat(267, 10, s.Title, "", 0, "L", false, 0, "")
		var paragraphs []string
		if s.Kind == "tasks" {
			// Preserve original descriptions even if the model shortened them.
			for _, task := range tasks {
				paragraphs = append(paragraphs, task.Key+" — "+task.Summary+"\n"+task.Description)
			}
		} else {
			for _, b := range s.Bullets {
				text := b.Text
				if len(b.SourceKeys) > 0 {
					text += " [" + strings.Join(b.SourceKeys, ", ") + "]"
				}
				paragraphs = append(paragraphs, text)
			}
		}
		if len(paragraphs) == 0 {
			switch s.Kind {
			case "title":
				paragraphs = []string{"Результаты по завершённым демо-задачам.\nСинтетические данные; документ не является официальным материалом Авито."}
			case "learning":
				paragraphs = []string{"Недостаточно подтверждённых данных о навыках."}
			default:
				paragraphs = []string{"Предложение: обсудить следующие задачи с наставником."}
			}
		}
		text := strings.Join(paragraphs, "\n\n")
		fits := false
		for size := 14.0; size >= 7; size-- {
			pdf.SetFont("GoRegular", "", size)
			lineHeight := size * 0.352778 * 1.35
			lines := pdf.SplitText(text, 267)
			if float64(len(lines))*lineHeight <= 111 {
				pdf.SetXY(15, 38)
				pdf.MultiCell(267, lineHeight, text, "", "L", false)
				fits = true
				break
			}
		}
		if !fits {
			return nil, errors.New("presentation content exceeds readable five-slide layout")
		}
		pdf.SetFont("GoRegular", "", 8)
		pdf.SetTextColor(100, 100, 110)
		pdf.SetXY(15, 155)
		pdf.CellFormat(267, 5, fmt.Sprintf("Демо-черновик · Синтетические данные · Чугун · %d / 5", i+1), "", 0, "L", false, 0, "")
	}
	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
