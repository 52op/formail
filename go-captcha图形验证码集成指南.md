# go-captcha v2 图形验证码集成指南

## 目录

1. [概述](#1-概述)
2. [五种验证模式](#2-五种验证模式)
3. [后端集成](#3-后端集成)
4. [前端集成](#4-前端集成)
5. [各模式实现详解](#5-各模式实现详解)
6. [问题根因复盘](#6-问题根因复盘)
7. [避坑清单](#7-避坑清单)
8. [快速集成步骤](#8-快速集成步骤)

---

## 1. 概述

go-captcha v2 (github.com/wenlng/go-captcha/v2) 是一套 Go 语言图形验证码库，提供五种验证模式。本文档基于 **v2.0.5** 实际踩坑经验整理。

核心依赖：
```
github.com/wenlng/go-captcha/v2          v2.0.5
github.com/wenlng/go-captcha-assets       (资源包：字体、背景图、滑块图等)
github.com/golang/freetype/truetype       (字体渲染)
```

### 架构要点

go-captcha 采用 **Builder + Generate** 模式：
- 全局初始化 **一个 Builder 实例**（每种模式一个），设置资源和参数
- 每次请求调用 `builder.Make()` 或 `builder.MakeShape()` / `builder.MakeDragDrop()` 生成具体 Captcha 实例
- 调 `.Generate()` 得到图片数据，从中提取 master 图、thumb 图、答案数据

---

## 2. 五种验证模式

| 模式 | 常量 | Builder 类型 | 创建方法 | 说明 |
|------|------|-------------|---------|------|
| 文字点选 | `click_text` | `click.Builder` | `builder.Make()` | 用户点击图中汉字，按顺序点选 |
| 图形点选 | `click_shape` | `click.Builder` | `builder.MakeShape()` | 用户点击图中图形，按顺序点选 |
| 滑动滑块 | `slide` | `slide.Builder` | `builder.Make()` | 拖动底部滑块，拼图块水平移动 |
| 拖拽拼图 | `drag` | `slide.Builder` | `builder.MakeDragDrop()` | 直接在背景图上拖拽拼图块，**2个拼图孔** |
| 旋转验证 | `rotate` | `rotate.Builder` | `builder.Make()` | 旋转内层图片，匹配外层背景 |

> **关键**：slide 和 drag 共用 `slide.Builder`，但 drag 需要额外配置 `slide.WithGenGraphNumber(2)`。

---

## 3. 后端集成

### 3.1 Builder 初始化

每种模式必须初始化一个**全局 Builder**，只初始化一次：

```go
var clickBuilder click.Builder   // 文字点选 + 图形点选共用
var slideBuilder slide.Builder   // 滑动滑块
var dragBuilder slide.Builder    // 拖拽拼图（注意：类型是 slide.Builder）
var rotateBuilder rotate.Builder // 旋转验证

func InitCaptchaBuilders() error {
    // 加载资源
    font, _ := fzshengsksjw.GetFont()
    bgImages, _ := imagesV2.GetImages()
    shapeImages, _ := shapes.GetShapes()
    tileImages, _ := tiles.GetTiles()
    
    // 转换 tileImages 为 GraphImage 切片
    slideGraphs := make([]*slide.GraphImage, 0, len(tileImages))
    for _, t := range tileImages {
        if t == nil { continue }
        slideGraphs = append(slideGraphs, &slide.GraphImage{
            OverlayImage: t.OverlayImage,
            ShadowImage:  t.ShadowImage,
            MaskImage:    t.MaskImage,
        })
    }

    // 1) 点选 Builder（文字 + 图形）
    clickBuilder = click.NewBuilder(
        click.WithImageSize(option.Size{Width: 320, Height: 200}),
        click.WithRangeLen(option.RangeVal{Min: 4, Max: 4}),
        click.WithRangeVerifyLen(option.RangeVal{Min: 4, Max: 4}),
    )
    clickBuilder.SetResources(
        click.WithChars(chars.GetChineseChars()),
        click.WithShapes(shapeImages),
        click.WithFonts([]*truetype.Font{font}),
        click.WithBackgrounds(bgImages),
    )

    // 2) 滑动滑块 Builder
    slideBuilder = slide.NewBuilder(
        slide.WithImageSize(option.Size{Width: 300, Height: 200}),
    )
    slideBuilder.SetResources(
        slide.WithBackgrounds(bgImages),
        slide.WithGraphImages(slideGraphs),
    )

    // 3) 拖拽拼图 Builder — ⚠️ 关键：WithGenGraphNumber(2)
    dragBuilder = slide.NewBuilder(
        slide.WithImageSize(option.Size{Width: 300, Height: 200}),
        slide.WithGenGraphNumber(2), // 背景上生成 2 个拼图孔
    )
    dragBuilder.SetResources(
        slide.WithBackgrounds(bgImages),
        slide.WithGraphImages(slideGraphs),
    )

    // 4) 旋转 Builder
    rotateBuilder = rotate.NewBuilder(
        rotate.WithImageSquareSize(220), // 主图 220x220
    )
    rotateBuilder.SetResources(
        rotate.WithImages(bgImages),
    )
    return nil
}
```

### 3.2 生成验证码

统一生成函数，根据 mode 分发：

```go
type Captcha struct {
    ID          string `json:"captcha_id"`
    Type        string `json:"captcha_type"`
    Base64Data  string `json:"captcha_base64"`  // 主图
    ThumbBase64 string `json:"thumb_base64"`    // 缩略图/拼图块
    TTL         int    `json:"ttl_seconds"`
    Hint        string `json:"hint"`
    // 点选模式
    Chars       string `json:"chars"`
    // 滑动/拖拽模式
    ThumbX      int    `json:"thumb_x"`
    ThumbY      int    `json:"thumb_y"`
    ThumbWidth  int    `json:"thumb_width"`
    ThumbHeight int    `json:"thumb_height"`
    InitX       int    `json:"init_x"`    // 拖拽模式初始 X
    InitY       int    `json:"init_y"`    // 拖拽模式初始 Y
    // 旋转模式
    Angle       int    `json:"angle"`      // 旋转角度
}
```

### 3.3 各模式的数据提取

**点选模式**（click_text / click_shape）：
```go
data, _ := capt.Generate()
masterImg := data.GetMasterImage()
masterData, _ := masterImg.ToBase64Data()
masterBase64 := "data:image/jpeg;base64," + masterData  // 点选模式是 JPEG

thumbImg := data.GetThumbImage()
thumbData, _ := thumbImg.ToBase64Data()
thumbBase64 := "data:image/png;base64," + thumbData     // thumbnail 是 PNG

dots := data.GetData()
// dots[i].X, dots[i].Y, dots[i].Text (文字) 或 dots[i].Shape (图形)
// 答案格式: "x1,y1,x2,y2,..."
```

**滑动/拖拽模式**（slide / drag）：
```go
data, _ := capt.Generate()
masterImg := data.GetMasterImage()
masterData, _ := masterImg.ToBase64Data()
masterBase64 := "data:image/jpeg;base64," + masterData  // 是 JPEG

tileImg := data.GetTileImage()
tileData, _ := tileImg.ToBase64Data()
tileBase64 := "data:image/png;base64," + tileData       // tile 是 PNG

block := data.GetData()
// block.X, block.Y — 目标位置
// block.Width, block.Height — 拼图块尺寸
// block.DX, block.DY — 拖拽模式初始位置偏移
// 答案格式(slide): "slide:block.X,block.Y" → 前端只传 X
// 答案格式(drag): "drag:block.X,block.Y" → 前端传 X,Y
```

**旋转模式**（rotate）：
```go
data, _ := capt.Generate()
masterImg := data.GetMasterImage()
masterData, _ := masterImg.ToBase64Data()
masterBase64 := "data:image/png;base64," + masterData  // ⚠️ 是 PNG！不是 JPEG！

thumbImg := data.GetThumbImage()
thumbData, _ := thumbImg.ToBase64Data()
thumbBase64 := "data:image/png;base64," + thumbData    // 是 PNG

block := data.GetData()
// block.Angle — 旋转角度
// block.Width, block.Height — thumb 图片尺寸
// 答案格式: "rotate:block.Angle"
```

> ⚠️ **大坑：旋转模式 master 是 PNG 而非 JPEG**。点选和滑动模式的 master 都是 JPEG，但旋转模式的 `data.GetMasterImage()` 返回 `imagedata.PNGImageData`，MIME type 必须是 `data:image/png;base64,`。

### 3.4 答案存储与验证

**存储**：将答案数据作为 `answer_hash` 存入数据库，标记 scene、IP、过期时间。

```go
db.Exec(`INSERT INTO captcha_challenges(id,scene,answer_hash,ip,expires_at) VALUES(?,?,?,?,?)`,
    id, scene, rawData, ip, expiresAt)
```

**验证**：从数据库取出原始数据，根据格式判断模式，分别验证：

```go
func VerifyCaptcha(db *sql.DB, id, answer, ip string) (bool, error) {
    // 1. 查询记录，检查 used==0 且未过期，IP 匹配
    // 2. 根据 rawData 前缀判断模式：
    
    if strings.HasPrefix(rawData, "[") {
        // 点选模式：JSON 数组 [{X,Y,Char,Index},...]
        // 每个点的坐标误差容忍 30px
    } else if strings.HasPrefix(rawData, "slide:") {
        // 滑动模式：slide.Validate(actualX, actualY, expectedX, expectedY, 40)
    } else if strings.HasPrefix(rawData, "drag:") {
        // 拖拽模式：slide.Validate(actualX, actualY, expectedX, expectedY, 55)
        // ⚠️ padding 比 slide 大（55 vs 40）
    } else if strings.HasPrefix(rawData, "rotate:") {
        // 旋转模式：rotate.Validate(actualAngle, expectedAngle, 30)
        // ⚠️ Validate 检查 (actualAngle + expectedAngle) 是否接近 360
    }
    // 3. 标记 used=1
}
```

> ⚠️ **关键**：`rotate.Validate(actual, expected, padding)` 的验证逻辑是检查 `(actual + expected)` 是否落在 `[360-padding, 360+padding]` 范围内。所以前端必须传 **补角** `(360 - 用户旋转角度) % 360`，不能直接传用户旋转角度。

---

## 4. 前端集成

### 4.1 API 响应字段

后端返回的 JSON 中**所有字段都必须传递给前端**，缺少任何字段都会导致对应模式异常：

```json
{
  "captcha_enabled": true,
  "captcha_id": "...",
  "captcha_type": "click_text|click_shape|slide|drag|rotate",
  "captcha_base64": "data:image/...;base64,...",
  "thumb_base64": "data:image/png;base64,...",
  "ttl_seconds": 180,
  "hint": "请依次点击图中的字符",
  "chars": "文字序列",
  "thumb_x": 0, "thumb_y": 0,
  "thumb_width": 0, "thumb_height": 0,
  "init_x": 0, "init_y": 0,
  "angle": 0
}
```

> ⚠️ `init_x`/`init_y` 是拖拽模式的初始位置，来自于 `block.DX`/`block.DY`。后端如果不在 handler 中手动写入这两个字段，前端拖拽模式的拼图块将始终从 (0,0) 开始，而不是从正确位置开始。

### 4.2 图片缩放计算

前端 canvas（`<img>` 标签）通过 CSS `width: 100%` 自适应容器宽度，实际显示尺寸 ≠ 图片自然尺寸。所有坐标换算都必须乘以缩放比：

```javascript
function getImageScale() {
    var nw = canvas.naturalWidth || 1;
    var dw = canvas.offsetWidth || nw;
    return dw / nw;
}

// 自然坐标 → 显示坐标
var displayX = naturalX * scale;
var displayY = naturalY * scale;

// 显示坐标 → 自然坐标（用于答案提交）
var naturalX = Math.round(displayX / scale);
var naturalY = Math.round(displayY / scale);
```

### 4.3 通用弹窗结构

```html
<div class="captcha-canvas-container">
    <img id="captchaCanvas" alt="验证码" />
</div>
<div id="captchaSliderArea" style="display:none;"></div>  <!-- 滑动条区域 -->
<input id="captchaAnswer" />  <!-- 隐藏的答案字段 -->
```

---

## 5. 各模式实现详解

### 5.1 文字点选 (click_text) / 图形点选 (click_shape)

**前端关键点**：
- Canvas 显示 master 图
- 显示 thumb 缩略图作为提示（"请依次点击：山 水 火 木"）
- 用户点击 canvas，记录点击坐标
- 在点击位置放置序号圆点标记
- 答案格式：`"x1,y1,x2,y2,x3,y3,x4,y4"`（按点击顺序）
- 后端容忍每个点 ±30px 误差

**无需滑块条**。

### 5.2 滑动滑块 (slide)

**前端关键点**：
- Canvas 显示 master 图
- 拼图块（thumb 图）初始位于左侧边缘（`initDispX = 0, initDispY = targetDispY`）
- 拼图块 pointer-events: none（不可直接拖拽）
- **显示底部滑块条**（`.captcha-slider`），拖动滑块→拼图块水平移动
- 拖动滑块时，拼图块只在水平方向移动（`initDispY` 固定）
- 答案格式：`String(naturalX)`
- 松手时拼图块位置与目标距离 ≤ 5px → 吸附成功

**CSS**：
```css
.captcha-piece {
    position: absolute;
    z-index: 10;
    pointer-events: none;  /* 不可直接拖拽 */
}
```

### 5.3 拖拽拼图 (drag)

**前端关键点**：
- Canvas 显示 master 图（背景上有 **2 个拼图孔**）
- 拼图块初始位于服务端指定位置（`initDispX = initX * scale, initDispY = initY * scale`）
- 拼图块 pointer-events: **auto**（可直接拖拽），cursor: grab
- **不显示底部滑块条**——直接监听拼图块的 mousedown/touchstart 事件
- 拖拽过程中拼图块跟随鼠标/手指任意方向移动
- 答案格式：`"naturalX,naturalY"`
- 松手时拼图块位置与目标距离 ≤ 8px → 吸附成功，否则回弹到初始位置
- 后端 `slide.Validate(actualX, actualY, expectedX, expectedY, 55)` — **padding=55 比 slide 的 40 更大**

**事件处理**：
```javascript
// mousedown/touchstart → 记录起始位置
// mousemove/touchmove → 更新拼图块位置
// mouseup/touchend → 判断是否在目标位置
```

> ⚠️ **大坑：拖拽和滑动必须分支处理**。拖拽模式下拼图块可任意方向移动，答案包含 X 和 Y。滑动模式下拼图块只能水平移动（Y 固定），答案只包含 X。两者不可用同一套逻辑。**在 JS 中拖拽模式处理完后必须 `return`，否则会继续执行滑动模式的滑块条创建代码。**

### 5.4 旋转验证 (rotate)

这是**坑最多**的模式。

#### 5.4.1 理解 DrawWithNRGBA vs DrawWithCropCircle

这是旋转验证最核心的概念，不理解这个就无法正确实现：

```
原始背景图 (例如 300x300)
         ↓
    DrawWithNRGBA: 从背景图中裁剪出一个圆形区域
    → MASTER 图片: 220x220 的未旋转圆形裁剪图（正确方向！）
         ↓
    DrawWithCropCircle: 从 MASTER 中再裁剪一个更小的圆形，然后 ROTATE
    → THUMB 图片: ~150x150 的已旋转圆形图（方向被旋转了 block.Angle 度！）
```

| 图片 | 生成方法 | 是否旋转 | 角色 | MIME |
|------|---------|---------|------|------|
| Master | `DrawWithNRGBA` | **否** — 只是从背景裁剪圆形 | 静态背景参考 | `image/png` |
| Thumb | `DrawWithCropCircle` | **是** — 旋转了 `block.Angle` 度 | 需要用户旋转回来 | `image/png` |

#### 5.4.2 正确的 UI 布局

```
┌─────────────────────────────┐
│  .captcha-canvas-container  │  ← 容器，正方形（宽=高），background-image: master
│                             │
│     ┌───────────────┐       │
│     │               │       │
│     │   thumb 图片   │       │  ← Canvas（<img>），绝对定位居中，由用户旋转
│     │   (要旋转的)    │       │
│     │               │       │
│     └───────────────┘       │
│                             │
└─────────────────────────────┘
        ↓ 底部滑块条
┌─────────────────────────────┐
│ ←→  拖动滑块旋转图片        │
└─────────────────────────────┘
```

**容器设置**：
```javascript
var cw = container.offsetWidth;
container.style.height = cw + "px";  // 正方形
container.style.backgroundImage = "url('" + captcha.base64 + "')";  // Master
container.style.backgroundSize = "contain";
```

**Canvas 设置**：
```javascript
canvas.src = captcha.thumbBase64;  // Thumb
canvas.style.position = "absolute";
canvas.style.left = "50%";
canvas.style.top = "50%";

// Thumb 按比例缩放
var masterNatSize = 220;  // rotate.WithImageSquareSize(220)
var thumbNatW = captcha.thumbWidth || canvas.naturalWidth || 140;
var ratio = thumbNatW / masterNatSize;
var displayW = Math.round(cw * ratio);
canvas.style.width = displayW + "px";

// 居中 + 旋转：必须合并到一个 transform 中
canvas.style.transform = "translate(-50%, -50%) rotate(-" + currentAngle + "deg)";
```

> ⚠️ **大坑：transform 合并**。居中需要 `translate(-50%, -50%)`，旋转需要 `rotate(-Xdeg)`。如果分开设置会互相覆盖。必须合并：`transform: translate(-50%, -50%) rotate(-Xdeg)`。

#### 5.4.3 角度计算

```javascript
function setAngle(angle) {
    currentAngle = ((angle % 360) + 360) % 360;
    // CSS 旋转（counterclockwise）
    canvas.style.transform = "translate(-50%, -50%) rotate(-" + currentAngle + "deg)";
    // ⚠️ 答案必须是补角：answer = (360 - currentAngle) % 360
    answerEl.value = Math.round((360 - currentAngle) % 360);
}
```

**为什么答案是补角？**
- 后端 `rotate.Validate(answer, blockAngle, 30)` 检查 `answer + blockAngle ≈ 360`
- 当前端旋转角度 `currentAngle = blockAngle` 时，图片匹配
- 此时 `answer = 360 - blockAngle`，满足 `answer + blockAngle = 360`，验证通过

#### 5.4.4 验证成功判断

```javascript
var diff = Math.abs(currentAngle - targetAngle);
diff = Math.min(diff, 360 - diff);  // 角度环形距离
if (diff <= 10) { /* 吸附成功 */ }
```

---

## 6. 问题根因复盘

以下是本项目在实际集成中遇到的所有 bug：

### Bug 1：拖拽模式显示滑块条
- **现象**：drag 模式 UI 和 slide 完全一样，有底部滑块条
- **根因**：`initSlideCaptcha` 没有区分 slide 和 drag，统一进入滑动条创建逻辑
- **修复**：在 drag 分支末尾 `return`，跳过滑动条创建

### Bug 2：拖拽模式拼图块从 (0,0) 开始
- **现象**：拖拽拼图块初始位置在左上角而非正确位置
- **根因**：`Captcha` 结构体和 Handler 没有传递 `init_x`/`init_y`（来自 `block.DX`/`block.DY`）
- **修复**：在 `Captcha` 结构体添加 `InitX`/`InitY` 字段，后端 `captchaRawData` 添加对应字段，Handler 的 JSON 响应添加 `init_x`/`init_y`

### Bug 3：拖拽模式只有 1 个拼图孔
- **现象**：背景图上只有 1 个拼图孔（和 slide 一样）
- **根因**：drag builder 没有设置 `slide.WithGenGraphNumber(2)`
- **修复**：dragBuilder 初始化时添加 `slide.WithGenGraphNumber(2)`

### Bug 4：旋转模式图片太小
- **现象**：旋转图片只有 120x120，难以操作
- **根因**：`rotate.WithImageSquareSize(120)` 太小
- **修复**：改为 `rotate.WithImageSquareSize(220)`

### Bug 5：旋转模式 master 图片无法显示/颜色异常
- **现象**：master 图片渲染异常
- **根因**：master 图片 MIME type 写成 `data:image/jpeg`，但 `data.GetMasterImage()` 在 rotate 模式下返回 PNG 格式
- **修复**：rotate 模式的 masterBase64 改为 `data:image/png;base64,`

### Bug 6（核心 Bug）：旋转模式内外层图片角色颠倒
- **现象**：用户反馈"内外层的图片都无法匹配，官方是转动中间的图片去匹配外面的圆形背景，你这个是反的"
- **根因**：
  - `DrawWithNRGBA` 生成的 master 是**未旋转**的圆形裁剪（正确方向）
  - `DrawWithCropCircle` 生成的 thumb 是**旋转过的**较小圆形
  - 代码将 master 放在 canvas 上旋转，将 thumb 作为静态覆盖层——完全反了
  - 用户永远无法通过旋转 master 来匹配 thumb（因为 master 本身就是正确的参考方向）
- **修复**：master 作为容器静态背景，thumb 放在 canvas 上旋转

### Bug 7：旋转模式 thumb 尺寸错误
- **现象**：thumb 图片显示过大或过小，与 master 背景无法对齐
- **根因**：thumb 图片是 master 的一个子区域（约 150/220 = 68%），如果按 100% 宽度显示会放大
- **修复**：按 `thumbNaturalWidth / masterNaturalSize` 的比例缩放 thumb 显示尺寸

### Bug 8：旋转验证提示文字不匹配
- **现象**：旋转模式提示文字是通用文字
- **根因**：未根据模式设置专属提示
- **修复**：提示改为"请拖动滑块旋转图片至正确角度"，滑块初始文字为"拖动滑块旋转图片"

### Bug 9：CSS transform 被覆盖
- **现象**：设置居中 transform 后再设置旋转，居中丢失
- **根因**：先后设置 `style.transform` 会互相覆盖
- **修复**：合并为 `transform: translate(-50%, -50%) rotate(-Xdeg)`

### Bug 10：清理函数不完整
- **现象**：切换验证码类型时，上次的样式残留
- **根因**：`cleanupDynamicWidgets` 没有重置容器背景图、高度和 canvas 绝对定位等样式
- **修复**：在清理函数中重置所有被修改的内联样式

---

## 7. 避坑清单

### 后端坑

| # | 坑 | 说明 |
|---|-----|------|
| 1 | 旋转 master 是 PNG | `data.GetMasterImage()` 在 rotate 模式返回 PNG，MIME 写 `image/png`。其他模式是 JPEG |
| 2 | drag 需要 2 个孔 | `slide.WithGenGraphNumber(2)`，否则背景只有 1 个孔 |
| 3 | drag 和 slide 共用 Builder | 类型都是 `slide.Builder`，需要两个实例 |
| 4 | `slide.Validate` 的 padding | drag 用 55，slide 用 40。太小会误拒，太大不安全 |
| 5 | `rotate.Validate` 逻辑 | 检查 `a + b ≈ 360`，不是 `a ≈ b`。前端必须传补角 |
| 6 | `block.DX`/`block.DY` 需要传给前端 | 拖拽模式初始位置，不加字段前端拿不到 |
| 7 | `block.Width`/`block.Height` 需要传给前端 | 旋转模式需要知道 thumb 尺寸来计算缩放比例 |
| 8 | 答案 hash 存储格式 | 每种模式格式不同，验证时根据前缀区分：`[`(JSON) / `slide:` / `drag:` / `rotate:` |

### 前端坑

| # | 坑 | 说明 |
|---|-----|------|
| 1 | drag 模式不要滑块条 | drag 分支末尾必须 `return`，否则会执行 slide 的滑块创建 |
| 2 | drag 模式 piece 可交互 | `pointer-events: auto`，`cursor: grab`，直接监听拖拽事件 |
| 3 | drag 模式 Y 轴可变 | 不固定 Y，答案包含 X,Y。slide 模式 Y 固定 |
| 4 | rotate 角色不能反 | Master = 静态背景，Thumb = 旋转层。反了就无法匹配 |
| 5 | rotate CSS transform 合并 | `translate(-50%, -50%) rotate(-Xdeg)` 必须在一条规则里 |
| 6 | rotate 答案用补角 | `answer = (360 - currentAngle) % 360` |
| 7 | rotate thumb 按比例缩放 | `displayWidth = containerWidth * (thumbNaturalW / 220)` |
| 8 | rotate 容器要正方形 | `container.style.height = container.offsetWidth + "px"` |
| 9 | 点选答案坐标顺序 | 必须按点击顺序排列：`x1,y1,x2,y2,...` |
| 10 | 坐标缩放 | 所有自然坐标 × scale = 显示坐标；显示坐标 / scale = 自然坐标 |
| 11 | 清理函数完整性 | 每次切换模式必须重置所有内联样式 |
| 12 | 图片加载时序 | `canvas.onload` 后再初始化交互逻辑，`canvas.complete` 时手动触发 |

---

## 8. 快速集成步骤

### Step 1: 安装依赖

```
go get github.com/wenlng/go-captcha/v2@v2.0.5
go get github.com/wenlng/go-captcha-assets/...
go get github.com/golang/freetype/truetype
```

### Step 2: 后端 — 初始化 Builder

参考 [3.1 节](#31-Builder-初始化)，创建 4 个全局 Builder（click、slide、drag、rotate），在服务启动时调用一次 `InitCaptchaBuilders()`。

### Step 3: 后端 — 生成接口

参考 [3.2 节](#32-生成验证码) 和 [3.3 节](#33-各模式的数据提取)，实现 `GenerateCaptcha()` 统一函数，根据 mode 分发到各生成函数。**确保 Captcha 结构体包含所有字段，Handler 返回所有字段**。

### Step 4: 后端 — 验证接口

参考 [3.4 节](#34-答案存储与验证)，实现 `VerifyCaptcha()`。根据存储的 rawData 前缀判断验证模式，调用对应的 Validate 函数。

### Step 5: 前端 — API 获取

`fetchCaptcha()` 从后端获取验证码数据，保存所有字段（特别是 `thumb_width`、`thumb_height`、`init_x`、`init_y`、`angle`）。

### Step 6: 前端 — 按模式初始化 UI

参考 [第 5 节](#5-各模式实现详解)，根据 `captcha.type` 分支：
- `click_text` / `click_shape` → 点选交互
- `slide` → 滑块条 + 拼图块水平移动
- `drag` → 拼图块直接拖拽（无滑块条），注意 `return`
- `rotate` → Master 静态背景 + Thumb 旋转层 + 滑块条

### Step 7: 前端 — 清理函数

参考 [Bug 10](#bug-10清理函数不完整)，确保 `cleanupDynamicWidgets()` 重置所有动态设置的内联样式。

### Step 8: 验证清单

- [ ] 5 种模式都能正常生成
- [ ] 5 种模式都能验证通过（正确操作）
- [ ] 5 种模式都能验证失败（错误操作）
- [ ] 过期验证码不能通过验证
- [ ] 已使用验证码不能再次使用
- [ ] 切换模式后 UI 无残留样式
- [ ] 刷新验证码后新验证码可用
- [ ] 移动端触摸事件正常
