package handler

import (
	"bytes"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
)

const (
	// MaxBookPages is the maximum number of pages in a book.
	MaxBookPages = 100
	// MaxPageLength is the maximum character count per page.
	MaxPageLength = 32767
	// MaxBookGeneration is the highest copy generation allowed (original=0, copy=1, copy-of-copy=2, tattered=3).
	MaxBookGeneration = 3
)

// BookManager handles book & quill editing, signing, and copying.
type BookManager struct{}

// NewBookManager creates a new BookManager.
func NewBookManager() *BookManager {
	return &BookManager{}
}

// HandlePacket processes ServerboundEditBook packets.
// Returns true if the packet was consumed.
func (bm *BookManager) HandlePacket(player *game.Player, p pk.Packet) bool {
	if packetid.ServerboundPacketID(p.ID) != packetid.ServerboundEditBook {
		return false
	}
	bm.handleEditBook(player, p)
	return true
}

// handleEditBook processes the ServerboundEditBook packet.
// Wire format: VarInt(slot) + VarInt(pageCount) + pageCount*String(page) + Optional[String(title)]
// If title is present, the book is being signed.
func (bm *BookManager) handleEditBook(player *game.Player, p pk.Packet) {
	slotIdx, pages, title, hasTitle, ok := parseEditBookPacket(p)
	if !ok {
		return
	}

	// Client sends the hotbar index (0-8), map to inventory slot
	inventorySlot := 36 + int(slotIdx)
	if inventorySlot < 36 || inventorySlot > 44 {
		return
	}

	held := &player.Inventory[inventorySlot]
	if held.ID == 0 || held.Count <= 0 {
		return
	}

	itemName := ItemNameByID(held.ID)
	if itemName != "writable_book" {
		return
	}

	// Truncate oversized pages
	for i, page := range pages {
		if len(page) > MaxPageLength {
			pages[i] = page[:MaxPageLength]
		}
	}

	if hasTitle {
		bm.signBook(player, inventorySlot, pages, title)
	} else {
		bm.editBook(player, inventorySlot, pages)
	}
}

// parseEditBookPacket extracts slot, pages, and optional title from the raw packet.
func parseEditBookPacket(p pk.Packet) (slot int32, pages []string, title string, hasTitle bool, ok bool) {
	r := bytes.NewReader(p.Data)

	var slotIdx pk.VarInt
	if _, err := slotIdx.ReadFrom(r); err != nil {
		return 0, nil, "", false, false
	}

	var pageCount pk.VarInt
	if _, err := pageCount.ReadFrom(r); err != nil {
		return 0, nil, "", false, false
	}
	if int(pageCount) < 0 || int(pageCount) > MaxBookPages {
		return 0, nil, "", false, false
	}

	pages = make([]string, int(pageCount))
	for i := 0; i < int(pageCount); i++ {
		var s pk.String
		if _, err := s.ReadFrom(r); err != nil {
			return 0, nil, "", false, false
		}
		pages[i] = string(s)
	}

	// Optional title (Boolean prefix)
	var hasOpt pk.Boolean
	if _, err := hasOpt.ReadFrom(r); err != nil {
		// No optional present — treat as edit-only
		return int32(slotIdx), pages, "", false, true
	}
	if bool(hasOpt) {
		var t pk.String
		if _, err := t.ReadFrom(r); err != nil {
			return int32(slotIdx), pages, "", false, true
		}
		title = string(t)
		if len(title) > 128 {
			title = title[:128]
		}
		return int32(slotIdx), pages, title, true, true
	}

	return int32(slotIdx), pages, "", false, true
}

// editBook updates the pages of a writable_book in the player's inventory.
func (bm *BookManager) editBook(player *game.Player, slot int, pages []string) {
	held := &player.Inventory[slot]
	held.BookPages = pages
	held.BookAuthor = ""
	held.BookTitle = ""
	held.BookGeneration = 0
}

// signBook converts a writable_book to a written_book with author, title, and pages.
func (bm *BookManager) signBook(player *game.Player, slot int, pages []string, title string) {
	writtenBookID := itemIDByName("written_book")
	if writtenBookID <= 0 {
		return
	}

	held := &player.Inventory[slot]
	held.ID = writtenBookID
	held.BookPages = pages
	held.BookAuthor = player.Name
	held.BookTitle = title
	held.BookGeneration = 0 // original

	// Sync the slot to the client
	SendSlotUpdate(player, slot)
}

// HandleUseItem processes ServerboundUseItem to open the book editing UI
// when the player right-clicks with a writable_book or written_book in hand.
// Returns true if handled.
func (bm *BookManager) HandleUseItem(player *game.Player, p pk.Packet) bool {
	if packetid.ServerboundPacketID(p.ID) != packetid.ServerboundUseItem {
		return false
	}

	var hand pk.VarInt
	var seq pk.VarInt
	if err := p.Scan(&hand, &seq); err != nil {
		return false
	}

	slotIdx := 36 + int(player.HeldSlot)
	if int(hand) == 1 {
		slotIdx = 45 // offhand
	}
	if slotIdx < 0 || slotIdx >= len(player.Inventory) {
		return false
	}

	held := &player.Inventory[slotIdx]
	if held.ID == 0 || held.Count <= 0 {
		return false
	}

	itemName := ItemNameByID(held.ID)
	switch itemName {
	case "writable_book", "written_book":
		player.WritePacket(pk.Marshal(
			packetid.ClientboundOpenBook,
			pk.VarInt(hand),
		))
		return true
	default:
		return false
	}
}

// CopyBook creates a copy of a written_book. The source book must have
// generation < MaxBookGeneration. Returns a new ItemStack for the copy,
// or nil if copying is not allowed.
func CopyBook(source *game.ItemStack) *game.ItemStack {
	if source == nil || source.ID == 0 || source.Count <= 0 {
		return nil
	}
	itemName := ItemNameByID(source.ID)
	if itemName != "written_book" {
		return nil
	}
	if source.BookGeneration >= MaxBookGeneration {
		return nil
	}

	copyPages := make([]string, len(source.BookPages))
	copy(copyPages, source.BookPages)

	return &game.ItemStack{
		ID:             source.ID,
		Count:          1,
		BookPages:      copyPages,
		BookAuthor:     source.BookAuthor,
		BookTitle:      source.BookTitle,
		BookGeneration: source.BookGeneration + 1,
	}
}

// CraftBookCopy handles the crafting recipe: written_book + writable_book = copied book.
// It searches the player's crafting grid for a written_book and a writable_book.
// Returns the result item and true if the recipe matches.
func CraftBookCopy(ingredients []game.ItemStack) (*game.ItemStack, bool) {
	var source *game.ItemStack
	hasBlank := false

	for i := range ingredients {
		it := &ingredients[i]
		if it.ID == 0 || it.Count <= 0 {
			continue
		}
		name := ItemNameByID(it.ID)
		switch name {
		case "written_book":
			if source != nil {
				return nil, false // only one written_book allowed
			}
			source = it
		case "writable_book":
			if hasBlank {
				return nil, false // only one writable_book allowed
			}
			hasBlank = true
		default:
			return nil, false // foreign items
		}
	}

	if source == nil || !hasBlank {
		return nil, false
	}

	result := CopyBook(source)
	if result == nil {
		return nil, false
	}
	return result, true
}
