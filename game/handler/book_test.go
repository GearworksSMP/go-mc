package handler

import (
	"testing"

	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
)

func TestParseEditBookPacket_EditOnly(t *testing.T) {
	// Build a packet: VarInt(slot=0) + VarInt(pages=2) + String("page1") + String("page2") + Boolean(false)
	p := pk.Marshal(0x17,
		pk.VarInt(0),
		pk.VarInt(2),
		pk.String("Hello world"),
		pk.String("Page two"),
		pk.Boolean(false),
	)

	slot, pages, title, hasTitle, ok := parseEditBookPacket(p)
	if !ok {
		t.Fatal("parseEditBookPacket returned not ok")
	}
	if slot != 0 {
		t.Errorf("expected slot 0, got %d", slot)
	}
	if len(pages) != 2 {
		t.Fatalf("expected 2 pages, got %d", len(pages))
	}
	if pages[0] != "Hello world" {
		t.Errorf("page 0: %q", pages[0])
	}
	if pages[1] != "Page two" {
		t.Errorf("page 1: %q", pages[1])
	}
	if hasTitle {
		t.Error("expected hasTitle=false")
	}
	if title != "" {
		t.Errorf("expected empty title, got %q", title)
	}
}

func TestParseEditBookPacket_WithSigning(t *testing.T) {
	p := pk.Marshal(0x17,
		pk.VarInt(3),
		pk.VarInt(1),
		pk.String("Content"),
		pk.Boolean(true),
		pk.String("My Book Title"),
	)

	slot, pages, title, hasTitle, ok := parseEditBookPacket(p)
	if !ok {
		t.Fatal("parseEditBookPacket returned not ok")
	}
	if slot != 3 {
		t.Errorf("expected slot 3, got %d", slot)
	}
	if len(pages) != 1 {
		t.Fatalf("expected 1 page, got %d", len(pages))
	}
	if pages[0] != "Content" {
		t.Errorf("page 0: %q", pages[0])
	}
	if !hasTitle {
		t.Error("expected hasTitle=true")
	}
	if title != "My Book Title" {
		t.Errorf("expected title 'My Book Title', got %q", title)
	}
}

func TestParseEditBookPacket_TooManyPages(t *testing.T) {
	p := pk.Marshal(0x17,
		pk.VarInt(0),
		pk.VarInt(101), // exceeds MaxBookPages
	)

	_, _, _, _, ok := parseEditBookPacket(p)
	if ok {
		t.Error("expected not ok for too many pages")
	}
}

func TestParseEditBookPacket_EmptyPages(t *testing.T) {
	p := pk.Marshal(0x17,
		pk.VarInt(0),
		pk.VarInt(0),
		pk.Boolean(false),
	)

	slot, pages, _, hasTitle, ok := parseEditBookPacket(p)
	if !ok {
		t.Fatal("parseEditBookPacket returned not ok")
	}
	if slot != 0 {
		t.Errorf("expected slot 0, got %d", slot)
	}
	if len(pages) != 0 {
		t.Errorf("expected 0 pages, got %d", len(pages))
	}
	if hasTitle {
		t.Error("expected hasTitle=false")
	}
}

func TestCopyBook(t *testing.T) {
	writtenID := itemIDByName("written_book")
	if writtenID <= 0 {
		t.Skip("written_book item not found in registry")
	}

	source := &game.ItemStack{
		ID:             writtenID,
		Count:          1,
		BookPages:      []string{"Page 1", "Page 2"},
		BookAuthor:     "TestAuthor",
		BookTitle:      "TestTitle",
		BookGeneration: 0,
	}

	result := CopyBook(source)
	if result == nil {
		t.Fatal("CopyBook returned nil")
	}
	if result.BookGeneration != 1 {
		t.Errorf("expected generation 1, got %d", result.BookGeneration)
	}
	if result.BookAuthor != "TestAuthor" {
		t.Errorf("expected author 'TestAuthor', got %q", result.BookAuthor)
	}
	if result.BookTitle != "TestTitle" {
		t.Errorf("expected title 'TestTitle', got %q", result.BookTitle)
	}
	if len(result.BookPages) != 2 {
		t.Fatalf("expected 2 pages, got %d", len(result.BookPages))
	}

	// Verify independence from source
	result.BookPages[0] = "Modified"
	if source.BookPages[0] == "Modified" {
		t.Error("copy pages should be independent from source")
	}
}

func TestCopyBook_MaxGeneration(t *testing.T) {
	writtenID := itemIDByName("written_book")
	if writtenID <= 0 {
		t.Skip("written_book item not found in registry")
	}

	source := &game.ItemStack{
		ID:             writtenID,
		Count:          1,
		BookPages:      []string{"Page 1"},
		BookAuthor:     "Author",
		BookTitle:      "Title",
		BookGeneration: MaxBookGeneration,
	}

	result := CopyBook(source)
	if result != nil {
		t.Error("expected nil for max generation book")
	}
}

func TestCopyBook_NotWrittenBook(t *testing.T) {
	writableID := itemIDByName("writable_book")
	if writableID <= 0 {
		t.Skip("writable_book item not found in registry")
	}

	source := &game.ItemStack{
		ID:    writableID,
		Count: 1,
	}

	result := CopyBook(source)
	if result != nil {
		t.Error("expected nil for writable_book")
	}
}

func TestCraftBookCopy(t *testing.T) {
	writtenID := itemIDByName("written_book")
	writableID := itemIDByName("writable_book")
	if writtenID <= 0 || writableID <= 0 {
		t.Skip("book items not found in registry")
	}

	ingredients := []game.ItemStack{
		{ID: writtenID, Count: 1, BookPages: []string{"Hello"}, BookAuthor: "Auth", BookTitle: "Title", BookGeneration: 0},
		{ID: writableID, Count: 1},
	}

	result, ok := CraftBookCopy(ingredients)
	if !ok {
		t.Fatal("CraftBookCopy returned false")
	}
	if result == nil {
		t.Fatal("CraftBookCopy returned nil result")
	}
	if result.BookGeneration != 1 {
		t.Errorf("expected generation 1, got %d", result.BookGeneration)
	}
}

func TestCraftBookCopy_NoWrittenBook(t *testing.T) {
	writableID := itemIDByName("writable_book")
	if writableID <= 0 {
		t.Skip("writable_book item not found in registry")
	}

	ingredients := []game.ItemStack{
		{ID: writableID, Count: 1},
		{ID: writableID, Count: 1},
	}

	_, ok := CraftBookCopy(ingredients)
	if ok {
		t.Error("expected false for two writable books")
	}
}

func TestCraftBookCopy_ForeignItem(t *testing.T) {
	writtenID := itemIDByName("written_book")
	writableID := itemIDByName("writable_book")
	stoneID := itemIDByName("stone")
	if writtenID <= 0 || writableID <= 0 || stoneID <= 0 {
		t.Skip("items not found in registry")
	}

	ingredients := []game.ItemStack{
		{ID: writtenID, Count: 1, BookPages: []string{"Hello"}, BookAuthor: "Auth", BookTitle: "Title"},
		{ID: writableID, Count: 1},
		{ID: stoneID, Count: 1},
	}

	_, ok := CraftBookCopy(ingredients)
	if ok {
		t.Error("expected false when foreign item present")
	}
}
