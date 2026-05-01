package main

import (
	"testing"

	"github.com/alecthomas/assert/v2"
)

func TestWrapStoreContent(t *testing.T) {
	t.Parallel()
	printer := storeEntryPrinter{width: 80, labelWidth: 18}

	tests := []struct {
		name    string
		content string
		want    []string
	}{
		{
			name:    "empty content stays as single empty line",
			content: "",
			want: []string{
				"",
			},
		},
		{
			name:    "tabs are normalized to spaces",
			content: "left\tright",
			want: []string{
				"left right",
			},
		},
		{
			name:    "respects existing paragraph breaks",
			content: "first paragraph\n\nsecond paragraph",
			want: []string{
				"first paragraph",
				"",
				"second paragraph",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, printer.wrapContent(tt.content))
		})
	}
}

func TestStoreEntryPrinterContentWrapWidth(t *testing.T) {
	t.Parallel()
	const minWrapWidth = 20

	tests := []struct {
		name      string
		printer   storeEntryPrinter
		wantWidth int
	}{
		{
			name: "uses width minus label width",
			printer: storeEntryPrinter{
				width:      80,
				labelWidth: 18,
			},
			wantWidth: 62,
		},
		{
			name: "applies minimum width floor",
			printer: storeEntryPrinter{
				width:      10,
				labelWidth: 9,
			},
			wantWidth: minWrapWidth,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantWidth, tt.printer.contentWrapWidth())
		})
	}
}
