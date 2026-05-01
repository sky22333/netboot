package storage

import (
	"context"
	"errors"
)

func (s *Store) defaultMenus() []Menu {
	return []Menu{
		{MenuType: "uefi", Enabled: false, Prompt: "UEFI Boot Menu", TimeoutSeconds: 6, Items: []MenuItem{
			{SortOrder: 1, Title: "iPXE UEFI x64", BootFile: "ipxe-x86_64.efi", PXEType: "8002", ServerIP: "%tftpserver%", Enabled: true},
			{SortOrder: 2, Title: "Boot Local Disk", BootFile: "", PXEType: "0000", ServerIP: "0.0.0.0", Enabled: true},
		}},
		{MenuType: "ipxe", Enabled: true, Prompt: "iPXE Boot Menu", TimeoutSeconds: 6, Items: []MenuItem{
			{SortOrder: 1, Title: "Run boot.ipxe", BootFile: "%dynamicboot%=boot.ipxe", PXEType: "0001", ServerIP: "%tftpserver%", Enabled: true},
			{SortOrder: 2, Title: "netboot.xyz", BootFile: "https://boot.netboot.xyz", PXEType: "8005", ServerIP: "%tftpserver%", Enabled: true},
			{SortOrder: 3, Title: "Boot Local Disk", BootFile: "", PXEType: "0000", ServerIP: "0.0.0.0", Enabled: true},
		}},
	}
}

func (s *Store) ensureMenu(ctx context.Context, menu Menu) error {
	var id int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM boot_menus WHERE menu_type=?`, menu.MenuType).Scan(&id)
	if err == nil && id > 0 {
		return nil
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO boot_menus(menu_type,enabled,prompt,timeout_seconds,randomize_timeout) VALUES(?,?,?,?,?)`, menu.MenuType, boolInt(menu.Enabled), menu.Prompt, menu.TimeoutSeconds, boolInt(menu.RandomizeTimeout))
	if err != nil {
		return err
	}
	id, _ = res.LastInsertId()
	for _, item := range menu.Items {
		_, err = s.db.ExecContext(ctx, `INSERT INTO boot_menu_items(menu_id,sort_order,title,boot_file,pxe_type,server_ip,enabled) VALUES(?,?,?,?,?,?,?)`, id, item.SortOrder, item.Title, item.BootFile, item.PXEType, item.ServerIP, boolInt(item.Enabled))
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ListMenus(ctx context.Context) ([]Menu, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,menu_type,enabled,prompt,timeout_seconds,randomize_timeout FROM boot_menus WHERE menu_type IN ('uefi','ipxe') ORDER BY CASE menu_type WHEN 'uefi' THEN 1 WHEN 'ipxe' THEN 2 ELSE 9 END`)
	if err != nil {
		return nil, err
	}
	menus := []Menu{}
	for rows.Next() {
		var m Menu
		var enabled, randomize int
		if err := rows.Scan(&m.ID, &m.MenuType, &enabled, &m.Prompt, &m.TimeoutSeconds, &randomize); err != nil {
			_ = rows.Close()
			return nil, err
		}
		m.Enabled = enabled == 1
		m.RandomizeTimeout = randomize == 1
		menus = append(menus, m)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for i := range menus {
		items, err := s.listMenuItems(ctx, menus[i].ID)
		if err != nil {
			return nil, err
		}
		menus[i].Items = items
	}
	return menus, nil
}

func (s *Store) listMenuItems(ctx context.Context, menuID int64) ([]MenuItem, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,menu_id,sort_order,title,COALESCE(boot_file,''),COALESCE(pxe_type,''),COALESCE(server_ip,''),enabled FROM boot_menu_items WHERE menu_id=? ORDER BY sort_order,id`, menuID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []MenuItem{}
	for rows.Next() {
		var item MenuItem
		var enabled int
		if err := rows.Scan(&item.ID, &item.MenuID, &item.SortOrder, &item.Title, &item.BootFile, &item.PXEType, &item.ServerIP, &enabled); err != nil {
			return nil, err
		}
		item.Enabled = enabled == 1
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) SaveMenus(ctx context.Context, menus []Menu) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM boot_menu_items`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM boot_menus`); err != nil {
		return err
	}
	for _, menu := range menus {
		if !allowedMenuType(menu.MenuType) {
			return errors.New("菜单类型无效")
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO boot_menus(menu_type,enabled,prompt,timeout_seconds,randomize_timeout) VALUES(?,?,?,?,?)`, menu.MenuType, boolInt(menu.Enabled), menu.Prompt, menu.TimeoutSeconds, boolInt(menu.RandomizeTimeout))
		if err != nil {
			return err
		}
		menuID, _ := res.LastInsertId()
		for _, item := range menu.Items {
			_, err = tx.ExecContext(ctx, `INSERT INTO boot_menu_items(menu_id,sort_order,title,boot_file,pxe_type,server_ip,enabled) VALUES(?,?,?,?,?,?,?)`, menuID, item.SortOrder, item.Title, item.BootFile, item.PXEType, item.ServerIP, boolInt(item.Enabled))
			if err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func allowedMenuType(menuType string) bool {
	return menuType == "uefi" || menuType == "ipxe"
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
