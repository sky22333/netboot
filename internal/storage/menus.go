package storage

import (
	"context"
)

func (s *Store) defaultMenus() []Menu {
	return []Menu{
		{MenuType: "bios", Enabled: true, Prompt: "按 F8 进入 BIOS 启动菜单", TimeoutSeconds: 6, Items: []MenuItem{
			{SortOrder: 1, Title: "iPXE BIOS", BootFile: "ipxe.bios", PXEType: "8000", ServerIP: "%tftpserver%", Enabled: true},
			{SortOrder: 2, Title: "本地硬盘启动", BootFile: "", PXEType: "0000", ServerIP: "0.0.0.0", Enabled: true},
		}},
		{MenuType: "uefi", Enabled: true, Prompt: "按 F8 进入 UEFI 启动菜单", TimeoutSeconds: 6, Items: []MenuItem{
			{SortOrder: 1, Title: "iPXE UEFI", BootFile: "ipxe.efi", PXEType: "8002", ServerIP: "%tftpserver%", Enabled: true},
			{SortOrder: 2, Title: "本地硬盘启动", BootFile: "", PXEType: "0000", ServerIP: "0.0.0.0", Enabled: true},
		}},
		{MenuType: "ipxe", Enabled: true, Prompt: "iPXE 启动菜单", TimeoutSeconds: 6, Items: []MenuItem{
			{SortOrder: 1, Title: "列出可启动文件", BootFile: "%dynamicboot%=ipxefm", PXEType: "0001", ServerIP: "%tftpserver%", Enabled: true},
			{SortOrder: 2, Title: "netboot.xyz", BootFile: "https://boot.netboot.xyz", PXEType: "8005", ServerIP: "%tftpserver%", Enabled: true},
			{SortOrder: 3, Title: "本地硬盘启动", BootFile: "", PXEType: "0000", ServerIP: "0.0.0.0", Enabled: true},
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
	rows, err := s.db.QueryContext(ctx, `SELECT id,menu_type,enabled,prompt,timeout_seconds,randomize_timeout FROM boot_menus ORDER BY CASE menu_type WHEN 'bios' THEN 1 WHEN 'uefi' THEN 2 WHEN 'ipxe' THEN 3 ELSE 9 END`)
	if err != nil {
		return nil, err
	}
	var menus []Menu
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
	var items []MenuItem
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

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
