package app

import (
	"fmt"

	"novel/internal/item"
	"novel/internal/storage"
)

func (a *App) GetItemList(novelID int64, itemType, status, search string, page, size int) (*storage.PageResult[item.Item], error) {
	return a.item.ListByNovel(a.ctx, novelID, item.ListOptions{
		Page: page, Size: size, ItemType: itemType, Status: status, Search: search,
	})
}

func (a *App) GetItemDetail(itemID int64) (*item.Item, error) {
	return a.item.GetByID(a.ctx, itemID, a.settings.LastNovelID)
}

func (a *App) FindItems(novelID int64, query string) ([]item.Item, error) {
	return a.item.Search(a.ctx, novelID, query, 20)
}

func (a *App) CreateItem(novelID int64, name, itemType, grade, description, lore, ability string, ownerID, arcID *int64, narrativeRole string) (*item.Item, error) {
	if name == "" { return nil, fmt.Errorf("名称不能为空") }
	it := &item.Item{
		NovelID: novelID, Name: name, ItemType: itemType, Grade: grade,
		Description: description, Lore: lore, Ability: ability,
		NarrativeRole: narrativeRole,
	}
	if ownerID != nil { it.OwnerID = ownerID }
	if arcID != nil { it.ArcID = arcID }
	if err := a.item.Create(a.ctx, it); err != nil {
		return nil, fmt.Errorf("create item: %w", err)
	}
	return it, nil
}

func (a *App) UpdateItem(itemID int64, name, itemType, grade, description, lore, ability, status string, ownerID, arcID *int64, narrativeRole string) error {
	it, err := a.item.GetByID(a.ctx, itemID, a.settings.LastNovelID)
	if err != nil { return fmt.Errorf("item not found: %w", err) }
	if name != "" { it.Name = name }
	if itemType != "" { it.ItemType = itemType }
	if grade != "" { it.Grade = grade }
	if description != "" { it.Description = description }
	if lore != "" { it.Lore = lore }
	if ability != "" { it.Ability = ability }
	if status != "" { it.Status = status }
	if ownerID != nil { it.OwnerID = ownerID }
	if arcID != nil { it.ArcID = arcID }
	if narrativeRole != "" { it.NarrativeRole = narrativeRole }
	return a.item.Update(a.ctx, it)
}

func (a *App) DeleteItem(itemID int64) error {
	return a.item.Delete(a.ctx, itemID, a.settings.LastNovelID)
}
