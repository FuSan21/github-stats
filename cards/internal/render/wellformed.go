package render

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
)

// CheckWellFormed parses svg as XML, failing on any malformed markup. Tests use it on every card.
func CheckWellFormed(svg string) error {
	d := xml.NewDecoder(strings.NewReader(svg))
	d.Strict = true
	first := true
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("malformed svg: %w", err)
		}
		if el, ok := tok.(xml.StartElement); ok && first {
			if el.Name.Local != "svg" {
				return fmt.Errorf("root element is <%s>, not <svg>", el.Name.Local)
			}
			first = false
		}
	}
}
