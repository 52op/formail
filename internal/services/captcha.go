package services

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"formail/internal/utils"

	"github.com/golang/freetype/truetype"
	"github.com/wenlng/go-captcha-assets/bindata/chars"
	"github.com/wenlng/go-captcha-assets/resources/fonts/fzshengsksjw"
	imagesV2 "github.com/wenlng/go-captcha-assets/resources/images_v2"
	"github.com/wenlng/go-captcha-assets/resources/shapes"
	"github.com/wenlng/go-captcha-assets/resources/tiles"
	"github.com/wenlng/go-captcha/v2/base/option"
	"github.com/wenlng/go-captcha/v2/click"
	"github.com/wenlng/go-captcha/v2/rotate"
	"github.com/wenlng/go-captcha/v2/slide"
)

type CaptchaMode string

const (
	CaptchaModeClickText  CaptchaMode = "click_text"
	CaptchaModeClickShape CaptchaMode = "click_shape"
	CaptchaModeSlide      CaptchaMode = "slide"
	CaptchaModeDrag       CaptchaMode = "drag"
	CaptchaModeRotate     CaptchaMode = "rotate"
	CaptchaModeRandom     CaptchaMode = "random"
)

type Captcha struct {
	ID          string `json:"captcha_id"`
	Type        string `json:"captcha_type"`
	Base64Data  string `json:"captcha_base64"`
	ThumbBase64 string `json:"thumb_base64"`
	TTL         int    `json:"ttl_seconds"`
	Hint        string `json:"hint"`
	Chars       string `json:"chars"`
	ThumbX      int    `json:"thumb_x"`
	ThumbY      int    `json:"thumb_y"`
	ThumbWidth  int    `json:"thumb_width"`
	ThumbHeight int    `json:"thumb_height"`
	Angle       int    `json:"angle"`
	InitX       int    `json:"init_x"`
	InitY       int    `json:"init_y"`
}

type ClickData struct {
	X     int    `json:"x"`
	Y     int    `json:"y"`
	Char  string `json:"char"`
	Index int    `json:"index"`
}

func (m CaptchaMode) IsValid() bool {
	switch m {
	case CaptchaModeClickText, CaptchaModeClickShape, CaptchaModeSlide, CaptchaModeDrag, CaptchaModeRotate, CaptchaModeRandom:
		return true
	}
	return false
}

func (m CaptchaMode) GetEffectiveMode() CaptchaMode {
	if m == CaptchaModeRandom {
		rand.Seed(time.Now().UnixNano())
		return []CaptchaMode{CaptchaModeClickText, CaptchaModeClickShape, CaptchaModeSlide, CaptchaModeDrag, CaptchaModeRotate}[rand.Intn(5)]
	}
	return m
}

var clickBuilder click.Builder
var slideBuilder slide.Builder
var dragBuilder slide.Builder
var rotateBuilder rotate.Builder

func InitCaptchaBuilders() error {
	font, err := fzshengsksjw.GetFont()
	if err != nil {
		return fmt.Errorf("加载字体失败: %w", err)
	}

	bgImages, err := imagesV2.GetImages()
	if err != nil {
		return fmt.Errorf("加载背景图片失败: %w", err)
	}
	shapeImages, err := shapes.GetShapes()
	if err != nil {
		return fmt.Errorf("加载图形资源失败: %w", err)
	}
	tileImages, err := tiles.GetTiles()
	if err != nil {
		return fmt.Errorf("加载滑块资源失败: %w", err)
	}
	slideGraphs := make([]*slide.GraphImage, 0, len(tileImages))
	for _, t := range tileImages {
		if t == nil {
			continue
		}
		slideGraphs = append(slideGraphs, &slide.GraphImage{
			OverlayImage: t.OverlayImage,
			ShadowImage:  t.ShadowImage,
			MaskImage:    t.MaskImage,
		})
	}

	clickBuilder = click.NewBuilder(
		click.WithImageSize(option.Size{Width: 320, Height: 200}),
		click.WithRangeLen(option.RangeVal{Min: 4, Max: 4}),
		click.WithRangeVerifyLen(option.RangeVal{Min: 4, Max: 4}),
		click.WithDisplayShadow(true),
	)

	clickBuilder.SetResources(
		click.WithChars(chars.GetChineseChars()),
		click.WithShapes(shapeImages),
		click.WithFonts([]*truetype.Font{font}),
		click.WithBackgrounds(bgImages),
	)

	slideBuilder = slide.NewBuilder(
		slide.WithImageSize(option.Size{Width: 300, Height: 200}),
	)
	slideBuilder.SetResources(
		slide.WithBackgrounds(bgImages),
		slide.WithGraphImages(slideGraphs),
	)

	dragBuilder = slide.NewBuilder(
		slide.WithImageSize(option.Size{Width: 300, Height: 200}),
		slide.WithGenGraphNumber(2),
	)
	dragBuilder.SetResources(
		slide.WithBackgrounds(bgImages),
		slide.WithGraphImages(slideGraphs),
	)

	rotateBuilder = rotate.NewBuilder(
		rotate.WithImageSquareSize(220),
	)
	rotateBuilder.SetResources(
		rotate.WithImages(bgImages),
	)

	return nil
}

func GenerateCaptcha(db *sql.DB, ip, scene string, ttlSeconds int, mode CaptchaMode) (*Captcha, error) {
	if clickBuilder == nil {
		if err := InitCaptchaBuilders(); err != nil {
			return nil, err
		}
	}

	if ttlSeconds <= 0 {
		ttlSeconds = 180
	}

	effectiveMode := mode.GetEffectiveMode()

	var captchaType string
	var base64Data string
	var thumbBase64 string
	var hint string
	var charsStr string
	var rawData string
	var blockX, blockY, blockWidth, blockHeight, angle, initX, initY int

	switch effectiveMode {
	case CaptchaModeClickText:
		captchaType = "click_text"
		data, chars, raw, err := generateClickTextCaptcha()
		if err != nil {
			return nil, err
		}
		base64Data = data
		thumbBase64 = raw.ThumbBase64
		hint = "请依次点击图中的字符"
		charsStr = chars
		rawData = raw.Data

	case CaptchaModeClickShape:
		captchaType = "click_shape"
		data, chars, raw, err := generateClickShapeCaptcha()
		if err != nil {
			return nil, err
		}
		base64Data = data
		thumbBase64 = raw.ThumbBase64
		hint = "请依次点击图中的图形"
		charsStr = chars
		rawData = raw.Data

	case CaptchaModeSlide:
		captchaType = "slide"
		data, raw, err := generateSlideCaptcha()
		if err != nil {
			return nil, err
		}
		base64Data = data
		thumbBase64 = raw.ThumbBase64
		hint = "请向右拖动底部滑块完成拼图"
		rawData = raw.Data
		blockX = raw.BlockX
		blockY = raw.BlockY
		blockWidth = raw.BlockWidth
		blockHeight = raw.BlockHeight
		initX = raw.InitX
		initY = raw.InitY

	case CaptchaModeDrag:
		captchaType = "drag"
		data, raw, err := generateDragCaptcha()
		if err != nil {
			return nil, err
		}
		base64Data = data
		thumbBase64 = raw.ThumbBase64
		hint = "请拖曳图片中的拼图碎片到正确位置"
		rawData = raw.Data
		blockX = raw.BlockX
		blockY = raw.BlockY
		blockWidth = raw.BlockWidth
		blockHeight = raw.BlockHeight
		initX = raw.InitX
		initY = raw.InitY

	case CaptchaModeRotate:
		captchaType = "rotate"
		data, raw, err := generateRotateCaptcha()
		if err != nil {
			return nil, err
		}
		base64Data = data
		thumbBase64 = raw.ThumbBase64
		hint = "请拖动底部滑块旋转图片至正确角度"
		rawData = raw.Data
		angle = raw.Angle
		blockWidth = raw.BlockWidth
		blockHeight = raw.BlockHeight

	default:
		return nil, fmt.Errorf("unsupported captcha mode")
	}

	id, err := utils.Token(18)
	if err != nil {
		return nil, err
	}

	expiresAt := time.Now().Add(time.Duration(ttlSeconds) * time.Second).Format(time.RFC3339)
	if _, err := db.Exec(`INSERT INTO captcha_challenges(id,scene,answer_hash,ip,expires_at) VALUES(?,?,?,?,?)`,
		id, scene, rawData, ip, expiresAt); err != nil {
		return nil, err
	}

	return &Captcha{
		ID:          id,
		Type:        captchaType,
		Base64Data:  base64Data,
		ThumbBase64: thumbBase64,
		TTL:         ttlSeconds,
		Hint:        hint,
		Chars:       charsStr,
		ThumbX:      blockX,
		ThumbY:      blockY,
		ThumbWidth:  blockWidth,
		ThumbHeight: blockHeight,
		Angle:       angle,
		InitX:       initX,
		InitY:       initY,
	}, nil
}

type captchaRawData struct {
	Answer      string
	ThumbBase64 string
	Data        string
	BlockX      int
	BlockY      int
	BlockWidth  int
	BlockHeight int
	Angle       int
	InitX       int
	InitY       int
}

func generateClickTextCaptcha() (string, string, *captchaRawData, error) {
	capt := clickBuilder.Make()
	if capt == nil {
		return "", "", nil, fmt.Errorf("生成文字点选验证码失败")
	}

	data, err := capt.Generate()
	if err != nil {
		return "", "", nil, err
	}

	masterImg := data.GetMasterImage()
	masterData, err := masterImg.ToBase64Data()
	if err != nil {
		return "", "", nil, err
	}
	masterBase64 := "data:image/jpeg;base64," + masterData

	thumbImg := data.GetThumbImage()
	thumbData, err := thumbImg.ToBase64Data()
	if err != nil {
		return "", "", nil, err
	}
	thumbBase64 := "data:image/png;base64," + thumbData

	dots := data.GetData()
	var answer string
	var chars string
	clickData := make([]ClickData, 0)

	for i := 0; i < len(dots); i++ {
		if i > 0 {
			answer += ","
			chars += " "
		}
		dot := dots[i]
		answer += fmt.Sprintf("%d,%d", dot.X, dot.Y)
		chars += dot.Text
		clickData = append(clickData, ClickData{
			X:     dot.X,
			Y:     dot.Y,
			Char:  dot.Text,
			Index: i + 1,
		})
	}

	rawData, _ := json.Marshal(clickData)

	return masterBase64, chars, &captchaRawData{
		Answer:      answer,
		ThumbBase64: thumbBase64,
		Data:        string(rawData),
	}, nil
}

func generateSlideCaptcha() (string, *captchaRawData, error) {
	capt := slideBuilder.Make()
	if capt == nil {
		return "", nil, fmt.Errorf("生成滑动验证码失败")
	}

	data, err := capt.Generate()
	if err != nil {
		return "", nil, err
	}

	masterImg := data.GetMasterImage()
	masterData, err := masterImg.ToBase64Data()
	if err != nil {
		return "", nil, err
	}
	masterBase64 := "data:image/jpeg;base64," + masterData

	tileImg := data.GetTileImage()
	tileData, err := tileImg.ToBase64Data()
	if err != nil {
		return "", nil, err
	}
	tileBase64 := "data:image/png;base64," + tileData

	block := data.GetData()

	return masterBase64, &captchaRawData{
		Answer:      fmt.Sprintf("%d", block.X),
		ThumbBase64: tileBase64,
		Data:        fmt.Sprintf("slide:%d,%d", block.X, block.Y),
		BlockX:      block.X,
		BlockY:      block.Y,
		BlockWidth:  block.Width,
		BlockHeight: block.Height,
		InitX:       block.DX,
		InitY:       block.DY,
	}, nil
}

func generateClickShapeCaptcha() (string, string, *captchaRawData, error) {
	capt := clickBuilder.MakeShape()
	if capt == nil {
		return "", "", nil, fmt.Errorf("生成图形点选验证码失败")
	}

	data, err := capt.Generate()
	if err != nil {
		return "", "", nil, err
	}

	masterImg := data.GetMasterImage()
	masterData, err := masterImg.ToBase64Data()
	if err != nil {
		return "", "", nil, err
	}
	masterBase64 := "data:image/jpeg;base64," + masterData

	thumbImg := data.GetThumbImage()
	thumbData, err := thumbImg.ToBase64Data()
	if err != nil {
		return "", "", nil, err
	}
	thumbBase64 := "data:image/png;base64," + thumbData

	dots := data.GetData()
	var answer string
	var shapes string
	clickData := make([]ClickData, 0)

	for i := 0; i < len(dots); i++ {
		if i > 0 {
			answer += ","
			shapes += " "
		}
		dot := dots[i]
		answer += fmt.Sprintf("%d,%d", dot.X, dot.Y)
		shapes += dot.Shape
		clickData = append(clickData, ClickData{
			X:     dot.X,
			Y:     dot.Y,
			Char:  dot.Shape,
			Index: i + 1,
		})
	}

	rawData, _ := json.Marshal(clickData)

	return masterBase64, shapes, &captchaRawData{
		Answer:      answer,
		ThumbBase64: thumbBase64,
		Data:        string(rawData),
	}, nil
}

func generateDragCaptcha() (string, *captchaRawData, error) {
	capt := dragBuilder.MakeDragDrop()
	if capt == nil {
		return "", nil, fmt.Errorf("生成拖拽滑块验证码失败")
	}

	data, err := capt.Generate()
	if err != nil {
		return "", nil, err
	}

	masterImg := data.GetMasterImage()
	masterData, err := masterImg.ToBase64Data()
	if err != nil {
		return "", nil, err
	}
	masterBase64 := "data:image/jpeg;base64," + masterData

	tileImg := data.GetTileImage()
	tileData, err := tileImg.ToBase64Data()
	if err != nil {
		return "", nil, err
	}
	tileBase64 := "data:image/png;base64," + tileData

	block := data.GetData()

	return masterBase64, &captchaRawData{
		Answer:      fmt.Sprintf("%d,%d", block.X, block.Y),
		ThumbBase64: tileBase64,
		Data:        fmt.Sprintf("drag:%d,%d", block.X, block.Y),
		BlockX:      block.X,
		BlockY:      block.Y,
		BlockWidth:  block.Width,
		BlockHeight: block.Height,
		InitX:       block.DX,
		InitY:       block.DY,
	}, nil
}

func generateRotateCaptcha() (string, *captchaRawData, error) {
	capt := rotateBuilder.Make()
	if capt == nil {
		return "", nil, fmt.Errorf("生成旋转验证码失败")
	}

	data, err := capt.Generate()
	if err != nil {
		return "", nil, err
	}

	masterImg := data.GetMasterImage()
	masterData, err := masterImg.ToBase64Data()
	if err != nil {
		return "", nil, err
	}
	masterBase64 := "data:image/png;base64," + masterData

	thumbImg := data.GetThumbImage()
	thumbData, err := thumbImg.ToBase64Data()
	if err != nil {
		return "", nil, err
	}
	thumbBase64 := "data:image/png;base64," + thumbData

	block := data.GetData()

	return masterBase64, &captchaRawData{
		Answer:      fmt.Sprintf("%d", block.Angle),
		ThumbBase64: thumbBase64,
		Data:        fmt.Sprintf("rotate:%d", block.Angle),
		Angle:       block.Angle,
		BlockWidth:  block.Width,
		BlockHeight: block.Height,
	}, nil
}

func getCaptchaFailureLimit(db *sql.DB) int {
	var v string
	if err := db.QueryRow(`SELECT value FROM settings WHERE key='captcha_failure_limit'`).Scan(&v); err != nil {
		return 5
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n <= 0 {
		return 5
	}
	return n
}

func VerifyCaptcha(db *sql.DB, id, answer, ip string) (bool, bool, error) {
	return VerifyCaptchaWithConsume(db, id, answer, ip, true)
}

func VerifyCaptchaWithConsume(db *sql.DB, id, answer, ip string, consume bool) (bool, bool, error) {
	var scene, rawData, storedIP, expiresAt string
	var used int
	var attempts int
	err := db.QueryRow(`SELECT scene,answer_hash,ip,used,expires_at,attempts FROM captcha_challenges WHERE id=?`, id).Scan(&scene, &rawData, &storedIP, &used, &expiresAt, &attempts)
	if err != nil {
		return false, false, err
	}
	if used == 1 {
		return false, false, nil
	}
	exp, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil || time.Now().After(exp) {
		return false, false, nil
	}
	if storedIP != "" && storedIP != ip {
		return false, false, nil
	}

	answer = strings.TrimSpace(answer)
	failureLimit := getCaptchaFailureLimit(db)
	incrAttempt := func() (bool, error) {
		attempts++
		if attempts >= failureLimit {
			if _, err := db.Exec(`UPDATE captcha_challenges SET attempts=?, used=1 WHERE id=?`, attempts, id); err != nil {
				return false, err
			}
			return true, nil
		}
		if _, err := db.Exec(`UPDATE captcha_challenges SET attempts=? WHERE id=?`, attempts, id); err != nil {
			return false, err
		}
		return false, nil
	}

	if strings.HasPrefix(rawData, "[") {
		var clickData []ClickData
		if err := json.Unmarshal([]byte(rawData), &clickData); err != nil {
			return false, false, nil
		}

		answerPoints := strings.Split(answer, ",")
		if len(answerPoints) != len(clickData)*2 {
			refreshRequired, err := incrAttempt()
			return false, refreshRequired, err
		}

		for i := 0; i < len(clickData); i++ {
			expectedX := clickData[i].X
			expectedY := clickData[i].Y
			actualX, _ := strconv.Atoi(answerPoints[i*2])
			actualY, _ := strconv.Atoi(answerPoints[i*2+1])

			dx := abs(expectedX - actualX)
			dy := abs(expectedY - actualY)
			if dx > 30 || dy > 30 {
				refreshRequired, err := incrAttempt()
				return false, refreshRequired, err
			}
		}
	} else if strings.HasPrefix(rawData, "slide:") || strings.HasPrefix(rawData, "drag:") {
		data := rawData
		if strings.HasPrefix(data, "slide:") {
			data = strings.TrimPrefix(data, "slide:")
		} else {
			data = strings.TrimPrefix(data, "drag:")
		}
		parts := strings.Split(data, ",")
		if len(parts) != 2 {
			refreshRequired, err := incrAttempt()
			return false, refreshRequired, err
		}
		expectedX, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
		expectedY, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err1 != nil || err2 != nil {
			refreshRequired, err := incrAttempt()
			return false, refreshRequired, err
		}

		answerParts := strings.Split(answer, ",")
		if len(answerParts) < 1 || strings.TrimSpace(answerParts[0]) == "" {
			refreshRequired, err := incrAttempt()
			return false, refreshRequired, err
		}
		actualX, err := strconv.Atoi(strings.TrimSpace(answerParts[0]))
		if err != nil {
			refreshRequired, err := incrAttempt()
			return false, refreshRequired, err
		}
		actualY := expectedY
		if len(answerParts) >= 2 && strings.TrimSpace(answerParts[1]) != "" {
			if ay, err := strconv.Atoi(strings.TrimSpace(answerParts[1])); err == nil {
				actualY = ay
			}
		}

		padding := 40
		if strings.HasPrefix(rawData, "drag:") {
			padding = 55
		}
		if !slide.Validate(actualX, actualY, expectedX, expectedY, padding) {
			refreshRequired, err := incrAttempt()
			return false, refreshRequired, err
		}
	} else if strings.HasPrefix(rawData, "rotate:") {
		expectedAngle, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(rawData, "rotate:")))
		if err != nil {
			refreshRequired, err := incrAttempt()
			return false, refreshRequired, err
		}
		actualAngle, err := strconv.Atoi(answer)
		if err != nil {
			refreshRequired, err := incrAttempt()
			return false, refreshRequired, err
		}
		if !rotate.Validate(actualAngle, expectedAngle, 30) {
			refreshRequired, err := incrAttempt()
			return false, refreshRequired, err
		}
	} else if strings.Contains(rawData, ",") {
		rawParts := strings.Split(rawData, ",")
		answerParts := strings.Split(answer, ",")
		if len(rawParts) != len(answerParts) {
			refreshRequired, err := incrAttempt()
			return false, refreshRequired, err
		}
		for i := 0; i < len(rawParts); i++ {
			expected, _ := strconv.Atoi(strings.TrimSpace(rawParts[i]))
			actual, _ := strconv.Atoi(strings.TrimSpace(answerParts[i]))
			if abs(expected-actual) > 30 {
				refreshRequired, err := incrAttempt()
				return false, refreshRequired, err
			}
		}
	} else {
		if answer != rawData {
			refreshRequired, err := incrAttempt()
			return false, refreshRequired, err
		}
	}

	if consume {
		_, _ = db.Exec(`UPDATE captcha_challenges SET used=1 WHERE id=?`, id)
	}
	return true, false, nil
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
