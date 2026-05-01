export type ServiceConfig = {
  server: { listen_ip: string; advertise_ip: string }
  dhcp: {
    enabled: boolean
    mode: string
    non_pxe_action: string
    pool_start: string
    pool_end: string
    subnet_mask: string
    router: string
    dns: string[]
    lease_time_seconds: number
    detect_conflicts: boolean
  }
  tftp: {
    enabled: boolean
    root: string
    allow_upload: boolean
    max_transfers: number
    block_size_max: number
    retry_count: number
    timeout_seconds: number
    max_upload_bytes: number
  }
  httpboot: {
    enabled: boolean
    addr: string
    root: string
    directory_listing: boolean
    range_requests: boolean
  }
  smb: { enabled: boolean; root: string; share_name: string; permissions: string }
  boot_files: {
    bios: string
    uefi_ia32: string
    uefi_x64: string
    uefi_arm32: string
    uefi_arm64: string
    ipxe: string
  }
  netboot_xyz: { enabled: boolean; download_dir: string; base_url: string; files: string[] }
  torrent: { enabled: boolean; addr: string }
  security: { admin_auth_enabled: boolean; allow_remote_admin: boolean }
}
