package handlers

import (
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"formail/internal/utils"

	"github.com/gin-gonic/gin"
)

type SiteSettings struct {
	SiteTitle        string `json:"site_title"`
	LogoURL          string `json:"logo_url"`
	FaviconURL       string `json:"favicon_url"`
	MetaTitle        string `json:"meta_title"`
	MetaDescription  string `json:"meta_description"`
	MetaKeywords     string `json:"meta_keywords"`
	Copyright        string `json:"copyright"`
	FooterContent    string `json:"footer_content"`
}

func getSiteSettings(db *sql.DB) (SiteSettings, error) {
	settings := SiteSettings{}
	settings.SiteTitle = getSetting(db, "site_title", "")
	settings.LogoURL = getSetting(db, "logo_url", "")
	settings.FaviconURL = getSetting(db, "favicon_url", "")
	settings.MetaTitle = getSetting(db, "meta_title", "")
	settings.MetaDescription = getSetting(db, "meta_description", "")
	settings.MetaKeywords = getSetting(db, "meta_keywords", "")
	settings.Copyright = getSetting(db, "copyright", "")
	settings.FooterContent = getSetting(db, "footer_content", "")
	return settings, nil
}

func (h *Handler) GetSiteSettings(c *gin.Context) {
	settings, err := getSiteSettings(h.DB)
	if err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	utils.OK(c, settings)
}

type SiteSettingsReq struct {
	SiteTitle       string `json:"site_title"`
	LogoURL         string `json:"logo_url"`
	FaviconURL      string `json:"favicon_url"`
	MetaTitle       string `json:"meta_title"`
	MetaDescription string `json:"meta_description"`
	MetaKeywords    string `json:"meta_keywords"`
	Copyright       string `json:"copyright"`
	FooterContent   string `json:"footer_content"`
}

func (h *Handler) UpdateSiteSettings(c *gin.Context) {
	var req SiteSettingsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Fail(c, 400, "请求格式无效")
		return
	}

	if len(req.SiteTitle) > 50 {
		utils.Fail(c, 400, "网站标题不能超过50个字符")
		return
	}
	if len(req.MetaTitle) > 60 {
		utils.Fail(c, 400, "元标题不能超过60个字符")
		return
	}
	if len(req.MetaDescription) > 160 {
		utils.Fail(c, 400, "元描述不能超过160个字符")
		return
	}

	oldLogo := getSetting(h.DB, "logo_url", "")
	oldFavicon := getSetting(h.DB, "favicon_url", "")

	settings := map[string]string{
		"site_title":        req.SiteTitle,
		"logo_url":          req.LogoURL,
		"favicon_url":       req.FaviconURL,
		"meta_title":        req.MetaTitle,
		"meta_description":  req.MetaDescription,
		"meta_keywords":     req.MetaKeywords,
		"copyright":        req.Copyright,
		"footer_content":   req.FooterContent,
	}

	for key, value := range settings {
		if _, err := h.DB.Exec(`INSERT OR REPLACE INTO settings(key, value) VALUES(?, ?)`, key, value); err != nil {
			utils.Fail(c, 500, fmt.Sprintf("保存设置 %s 失败: %v", key, err))
			return
		}
	}

	if oldLogo != "" && req.LogoURL != oldLogo {
		deleteOldFile(oldLogo)
	}
	if oldFavicon != "" && req.FaviconURL != oldFavicon {
		deleteOldFile(oldFavicon)
	}

	utils.OK(c, gin.H{"updated": true})
}

func (h *Handler) UploadSiteAsset(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		utils.Fail(c, 400, "请选择要上传的文件")
		return
	}

	fileType := c.PostForm("type")
	if fileType != "logo" && fileType != "favicon" {
		utils.Fail(c, 400, "无效的文件类型")
		return
	}

	ext := strings.ToLower(filepath.Ext(file.Filename))
	allowedExts := map[string]bool{
		".jpg":  true,
		".jpeg": true,
		".png":  true,
		".svg":  true,
		".ico":  true,
	}
	if !allowedExts[ext] {
		utils.Fail(c, 400, "不支持的文件格式，支持 JPG、PNG、SVG、ICO")
		return
	}

	if file.Size > 2*1024*1024 {
		utils.Fail(c, 400, "文件大小不能超过2MB")
		return
	}

	oldURL := getSetting(h.DB, fileType+"_url", "")

	timestamp := time.Now().Unix()
	newFileName := fmt.Sprintf("%s_%d%s", fileType, timestamp, ext)
	savePath := filepath.Join("web", "static", "uploads", newFileName)

	err = os.MkdirAll(filepath.Dir(savePath), 0755)
	if err != nil {
		utils.Fail(c, 500, "创建目录失败")
		return
	}

	dst, err := os.Create(savePath)
	if err != nil {
		utils.Fail(c, 500, "创建文件失败")
		return
	}
	defer dst.Close()

	src, err := file.Open()
	if err != nil {
		utils.Fail(c, 500, "打开文件失败")
		return
	}
	defer src.Close()

	if _, err := io.Copy(dst, src); err != nil {
		utils.Fail(c, 500, "保存文件失败")
		return
	}

	if oldURL != "" {
		deleteOldFile(oldURL)
	}

	url := "/assets/uploads/" + newFileName
	utils.OK(c, gin.H{"url": url})
}

func deleteOldFile(url string) {
	if !strings.HasPrefix(url, "/assets/uploads/") {
		return
	}
	filePath := filepath.Join("web", "static", "uploads", filepath.Base(url))
	os.Remove(filePath)
}