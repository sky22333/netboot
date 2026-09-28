package web

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"pxe/internal/firmware"
)

func (h *Handler) firmwareCatalog(c *gin.Context) {
	settings, err := h.app.Storage().GetSettings(c.Request.Context())
	if err != nil {
		Fail(c, 500, "CONFIG_READ_FAILED", "读取配置失败")
		return
	}
	sources, err := firmware.Catalog(settings)
	if err != nil {
		Fail(c, 500, "FIRMWARE_LIST_FAILED", "读取固件目录失败："+err.Error())
		return
	}
	OK(c, gin.H{"sources": sources})
}

func (h *Handler) firmwareDownload(c *gin.Context) {
	var req struct {
		Source string   `json:"source"`
		Files  []string `json:"files"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, 400, "REQUEST_INVALID", "请选择固件")
		return
	}
	settings, err := h.app.Storage().GetSettings(c.Request.Context())
	if err != nil {
		Fail(c, 500, "CONFIG_READ_FAILED", "读取配置失败")
		return
	}
	source, err := firmware.Select(settings, req.Source, req.Files)
	if err != nil {
		Fail(c, 400, "FIRMWARE_INVALID", err.Error())
		return
	}
	select {
	case h.firmwareSlots <- struct{}{}:
		defer func() { <-h.firmwareSlots }()
	default:
		Fail(c, 409, "FIRMWARE_BUSY", "已有固件下载任务，请稍后重试")
		return
	}
	results, err := firmware.Download(c.Request.Context(), &http.Client{Timeout: 90 * time.Second}, source, settings.NetbootXYZ.BaseURL, h.app.EventHub())
	if err != nil {
		Fail(c, 502, "FIRMWARE_DOWNLOAD_FAILED", err.Error())
		return
	}
	OK(c, gin.H{"downloads": results})
}
