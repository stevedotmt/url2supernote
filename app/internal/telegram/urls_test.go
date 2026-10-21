package telegram

import (
	"net/url"
	"slices"
	"testing"
)

func urlStrings(urls []*url.URL) []string {
	var s []string
	for _, u := range urls {
		s = append(s, u.String())
	}
	return s
}

func TestMessageURLs(t *testing.T) {
	tests := []struct {
		name string
		msg  Message
		want []string
	}{
		{
			name: "url entity after emoji uses UTF-16 offsets",
			msg: Message{
				Text:     "📚 Read https://example.com/a later",
				Entities: []Entity{{Type: "url", Offset: 8, Length: 21}},
			},
			want: []string{"https://example.com/a"},
		},
		{
			name: "url entity without scheme",
			msg: Message{
				Text:     "see example.com/post",
				Entities: []Entity{{Type: "url", Offset: 4, Length: 16}},
			},
			want: []string{"https://example.com/post"},
		},
		{
			name: "text link",
			msg: Message{
				Text:     "this article",
				Entities: []Entity{{Type: "text_link", Offset: 0, Length: 12, URL: "https://example.com/hidden"}},
			},
			want: []string{"https://example.com/hidden"},
		},
		{
			name: "share sheet caption with duplicates and non-URL entities",
			msg: Message{
				Caption: "Title\nhttps://example.com/x #tag https://example.com/x",
				CaptionEntities: []Entity{
					{Type: "url", Offset: 6, Length: 21},
					{Type: "hashtag", Offset: 28, Length: 4},
					{Type: "url", Offset: 33, Length: 21},
				},
			},
			want: []string{"https://example.com/x"},
		},
		{
			name: "plain text fallback trims trailing punctuation",
			msg:  Message{Text: "Two: https://a.example/1, and (https://b.example/2)."},
			want: []string{"https://a.example/1", "https://b.example/2"},
		},
		{
			name: "ignores other schemes and out-of-range entities",
			msg: Message{
				Text:     "ftp://example.com",
				Entities: []Entity{{Type: "url", Offset: 0, Length: 17}, {Type: "url", Offset: 10, Length: 99}},
			},
		},
		{
			name: "no URL",
			msg:  Message{Text: "hello"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := urlStrings(MessageURLs(&tt.msg)); !slices.Equal(got, tt.want) {
				t.Errorf("MessageURLs() = %q, want %q", got, tt.want)
			}
		})
	}
}
