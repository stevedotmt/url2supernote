package epub

import (
	"net/url"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

var keptElements = map[atom.Atom]bool{
	atom.A: true, atom.Abbr: true, atom.Article: true, atom.Aside: true, atom.B: true,
	atom.Blockquote: true, atom.Br: true, atom.Caption: true, atom.Cite: true, atom.Code: true,
	atom.Dd: true, atom.Del: true, atom.Div: true, atom.Dl: true, atom.Dt: true, atom.Em: true,
	atom.Figcaption: true, atom.Figure: true, atom.H1: true, atom.H2: true, atom.H3: true,
	atom.H4: true, atom.H5: true, atom.H6: true, atom.Hr: true, atom.I: true, atom.Img: true,
	atom.Ins: true, atom.Kbd: true, atom.Li: true, atom.Mark: true, atom.Ol: true, atom.P: true,
	atom.Pre: true, atom.Q: true, atom.S: true, atom.Samp: true, atom.Section: true,
	atom.Small: true, atom.Span: true, atom.Strong: true, atom.Sub: true, atom.Sup: true,
	atom.Table: true, atom.Tbody: true, atom.Td: true, atom.Tfoot: true, atom.Th: true,
	atom.Thead: true, atom.Time: true, atom.Tr: true, atom.U: true, atom.Ul: true, atom.Var: true,
}

var droppedElements = map[atom.Atom]bool{
	atom.Audio: true, atom.Button: true, atom.Canvas: true, atom.Embed: true, atom.Form: true,
	atom.Head: true, atom.Iframe: true, atom.Input: true, atom.Link: true, atom.Math: true,
	atom.Meta: true, atom.Noscript: true, atom.Object: true, atom.Script: true, atom.Select: true,
	atom.Source: true, atom.Style: true, atom.Svg: true, atom.Template: true, atom.Textarea: true,
	atom.Video: true,
}

var voidElements = map[atom.Atom]bool{atom.Br: true, atom.Hr: true, atom.Img: true}

func parseFragment(content string) ([]*html.Node, error) {
	body := &html.Node{Type: html.ElementNode, DataAtom: atom.Body, Data: "body"}
	return html.ParseFragment(strings.NewReader(content), body)
}

func imageURLs(nodes []*html.Node, base *url.URL) []*url.URL {
	var found []*url.URL
	seen := map[string]bool{}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.DataAtom == atom.Img {
			if u := resolveImage(n, base); u != nil && !seen[u.String()] {
				seen[u.String()] = true
				found = append(found, u)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	for _, n := range nodes {
		walk(n)
	}
	return found
}

func resolveImage(img *html.Node, base *url.URL) *url.URL {
	src := attr(img, "src")
	if src == "" {
		src = firstSrcset(attr(img, "srcset"))
	}
	return resolveHTTP(src, base)
}

func firstSrcset(srcset string) string {
	first, _, _ := strings.Cut(srcset, ",")
	fields := strings.Fields(first)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func resolveHTTP(ref string, base *url.URL) *url.URL {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil
	}
	parsed, err := url.Parse(ref)
	if err != nil {
		return nil
	}
	if base != nil {
		parsed = base.ResolveReference(parsed)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil
	}
	return parsed
}

type xhtmlWriter struct {
	b      strings.Builder
	base   *url.URL
	images map[string]string
}

func renderXHTML(nodes []*html.Node, base *url.URL, images map[string]string) string {
	w := &xhtmlWriter{base: base, images: images}
	for _, n := range nodes {
		w.node(n)
	}
	return w.b.String()
}

func (w *xhtmlWriter) node(n *html.Node) {
	switch n.Type {
	case html.TextNode:
		w.b.WriteString(escape(n.Data))
	case html.ElementNode:
		w.element(n)
	case html.DocumentNode:
		w.children(n)
	}
}

func (w *xhtmlWriter) children(n *html.Node) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		w.node(c)
	}
}

func (w *xhtmlWriter) element(n *html.Node) {
	if droppedElements[n.DataAtom] {
		return
	}
	if !keptElements[n.DataAtom] {
		w.children(n)
		return
	}

	attrs, ok := w.attributes(n)
	if !ok {
		return
	}

	w.b.WriteString("<" + n.Data)
	for _, a := range attrs {
		w.b.WriteString(" " + a.Key + `="` + escape(a.Val) + `"`)
	}
	if voidElements[n.DataAtom] {
		w.b.WriteString("/>")
		return
	}
	w.b.WriteString(">")
	w.children(n)
	w.b.WriteString("</" + n.Data + ">")
}

func (w *xhtmlWriter) attributes(n *html.Node) ([]html.Attribute, bool) {
	switch n.DataAtom {
	case atom.Img:
		u := resolveImage(n, w.base)
		if u == nil {
			return nil, false
		}
		local, ok := w.images[u.String()]
		if !ok {
			return nil, false
		}
		return []html.Attribute{{Key: "src", Val: local}, {Key: "alt", Val: attr(n, "alt")}}, true
	case atom.A:
		if href := resolveLink(attr(n, "href"), w.base); href != "" {
			return []html.Attribute{{Key: "href", Val: href}}, true
		}
		return nil, true
	case atom.Td, atom.Th:
		return keep(n, "colspan", "rowspan"), true
	case atom.Ol:
		return keep(n, "start"), true
	default:
		return nil, true
	}
}

func resolveLink(href string, base *url.URL) string {
	if strings.HasPrefix(strings.TrimSpace(href), "mailto:") {
		return strings.TrimSpace(href)
	}
	if u := resolveHTTP(href, base); u != nil {
		return u.String()
	}
	return ""
}

func keep(n *html.Node, keys ...string) []html.Attribute {
	var kept []html.Attribute
	for _, key := range keys {
		if v := attr(n, key); v != "" {
			kept = append(kept, html.Attribute{Key: key, Val: v})
		}
	}
	return kept
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Namespace == "" && a.Key == key {
			return a.Val
		}
	}
	return ""
}

var xmlEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;")

func escape(s string) string {
	return xmlEscaper.Replace(stripInvalidXMLChars(s))
}

func stripInvalidXMLChars(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\t' || r == '\n' || r == '\r':
			return r
		case r < 0x20, r == 0xFFFE, r == 0xFFFF, r >= 0xD800 && r <= 0xDFFF:
			return -1
		default:
			return r
		}
	}, s)
}
