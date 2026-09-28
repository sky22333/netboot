package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"time"
)

// LeaseAddress serializes reservations and durable leases in one transaction.
// An empty result means no address is available; database failures are returned.
func (s *Store) LeaseAddress(ctx context.Context, cfg ServiceSettings, mac, requested string, confirm bool) (string, error) {
	mac = NormalizeMAC(mac)
	if !validMAC(mac) {
		return "", fmt.Errorf("MAC 地址无效")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	// A committed transaction needs no rollback; preserve the original error otherwise.
	defer func() { _ = tx.Rollback() }()
	now := time.Now().Unix()
	if _, err = tx.ExecContext(ctx, `DELETE FROM leases WHERE expires<=?`, now); err != nil {
		return "", err
	}
	reserved := map[string]string{}
	rows, err := tx.QueryContext(ctx, `SELECT ip,COALESCE(mac,'') FROM clients WHERE ip IS NOT NULL AND ip!=''`)
	if err != nil {
		return "", err
	}
	staticIP := ""
	for rows.Next() {
		var ip, owner string
		if err = rows.Scan(&ip, &owner); err != nil {
			rows.Close()
			return "", err
		}
		reserved[ip] = owner
		if owner == mac {
			staticIP = ip
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return "", err
	}
	used := map[string]string{}
	rows, err = tx.QueryContext(ctx, `SELECT ip,mac FROM leases`)
	if err != nil {
		return "", err
	}
	for rows.Next() {
		var ip, owner string
		if err = rows.Scan(&ip, &owner); err != nil {
			rows.Close()
			return "", err
		}
		used[ip] = owner
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return "", err
	}
	mask := net.IPMask(net.ParseIP(cfg.DHCP.SubnetMask).To4())
	subnet := net.ParseIP(cfg.DHCP.Router).To4().Mask(mask)
	allowed := func(ip string) bool {
		v := net.ParseIP(ip).To4()
		if !usableIPv4(ip) || v == nil || !v.Mask(mask).Equal(subnet) {
			return false
		}
		n := binaryBig(v)
		network := binaryBig(subnet)
		m := binaryBig(net.IP(mask))
		if n == network || n == network|^m {
			return false
		}
		if ip == cfg.Server.AdvertiseIP || ip == cfg.DHCP.Router {
			return false
		}
		if owner, ok := reserved[ip]; ok && owner != mac {
			return false
		}
		if owner, ok := used[ip]; ok && owner != mac {
			return false
		}
		return true
	}
	inPool := func(ip string) bool {
		v := net.ParseIP(ip).To4()
		return v != nil && binaryBig(v) >= binaryBig(net.ParseIP(cfg.DHCP.PoolStart)) && binaryBig(v) <= binaryBig(net.ParseIP(cfg.DHCP.PoolEnd))
	}
	var previous string
	var expiry int64
	var confirmed int
	err = tx.QueryRowContext(ctx, `SELECT ip,expires,confirmed FROM leases WHERE mac=?`, mac).Scan(&previous, &expiry, &confirmed)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	target := ""
	if staticIP != "" {
		if allowed(staticIP) && (!confirm || requested == "" || requested == staticIP) {
			target = staticIP
		}
	} else if confirm && requested != "" {
		if inPool(requested) && allowed(requested) {
			target = requested
		}
	} else if previous != "" && inPool(previous) && allowed(previous) {
		target = previous
	} else if !confirm {
		if requested != "" && inPool(requested) && allowed(requested) {
			target = requested
		}
		if target == "" {
			for n, end := binaryBig(net.ParseIP(cfg.DHCP.PoolStart)), binaryBig(net.ParseIP(cfg.DHCP.PoolEnd)); n <= end; n++ {
				v := make(net.IP, 4)
				putBinary(v, n)
				if allowed(v.String()) {
					target = v.String()
					break
				}
				if n == ^uint32(0) {
					break
				}
			}
		}
	}
	if target == "" {
		return "", nil
	}
	until := now + 60
	isConfirmed := 0
	if confirm {
		until = now + int64(cfg.DHCP.LeaseTimeSeconds)
		isConfirmed = 1
	} else if target == previous && confirmed == 1 {
		until = expiry
		isConfirmed = 1
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO leases(mac,ip,expires,confirmed) VALUES(?,?,?,?) ON CONFLICT(mac) DO UPDATE SET ip=excluded.ip,expires=excluded.expires,confirmed=excluded.confirmed`, mac, target, until, isConfirmed)
	if err != nil {
		return "", err
	}
	if err = tx.Commit(); err != nil {
		return "", err
	}
	return target, nil
}

func (s *Store) ReleaseLease(ctx context.Context, mac, ip string, decline bool) error {
	mac = NormalizeMAC(mac)
	if decline {
		// Keep the address quarantined; it cannot be reassigned during the hold-down.
		_, err := s.db.ExecContext(ctx, `UPDATE leases SET mac='declined:'||ip,expires=unixepoch()+600,confirmed=0 WHERE mac=? AND ip=?`, mac, ip)
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM leases WHERE mac=? AND ip=?`, mac, ip)
	return err
}
