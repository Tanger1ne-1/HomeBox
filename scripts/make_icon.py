# 生成 HomeBox 图标 icon.ico（多尺寸 16/32/48/256）。
# 关键点：背景用圆角矩形 + 透明圆角，而不是铺满整个正方形的实心方块，
# 这样 Windows 桌面/任务栏/安装包里显示的是「圆边倒角」图标，而非尖角方块。
# 运行：py scripts/make_icon.py
from PIL import Image, ImageDraw

# 设计基准坐标系：512x512（与 icon.svg 完全一致）。
# 颜色
TOP = (79, 140, 255)      # #4f8cff
BOT = (47, 111, 224)      # #2f6fe0
WHITE = (255, 255, 255, 255)
DRAWER = (29, 78, 216, 255)   # #1d4ed8
ARROW = (47, 111, 224, 255)   # #2f6fe0

CORNER = 116               # 圆角半径（512 基准）


def lerp(a, b, t):
    return tuple(int(round(a[i] + (b[i] - a[i]) * t)) for i in range(3))


def draw_icon(size, ss=4):
    """按 512 基准绘制并超采样 ss 倍，再缩放到目标尺寸，边缘更平滑。"""
    S = size * ss
    scale = S / 512.0  # 512 基准坐标 → 超采样画布

    # 竖向渐变背景
    grad = Image.new("RGBA", (S, S), (0, 0, 0, 0))
    gpix = grad.load()
    for y in range(S):
        c = lerp(TOP, BOT, y / (S - 1))
        for x in range(S):
            gpix[x, y] = (*c, 255)

    # 圆角矩形遮罩：圆角外透明
    mask = Image.new("L", (S, S), 0)
    ImageDraw.Draw(mask).rounded_rectangle(
        [0, 0, S - 1, S - 1], radius=int(round(CORNER * scale)), fill=255
    )

    img = Image.new("RGBA", (S, S), (0, 0, 0, 0))
    img.paste(grad, (0, 0), mask)
    d = ImageDraw.Draw(img)

    def P(x, y):
        return (int(round(x * scale)), int(round(y * scale)))

    # 房子：屋顶三角 + 房身圆角矩形
    d.polygon([P(256, 92), P(104, 224), P(408, 224)], fill=WHITE)
    d.rounded_rectangle(
        [P(128, 224)[0], P(128, 224)[1], P(384, 384)[0], P(384, 384)[1]],
        radius=int(round(24 * scale)), fill=WHITE,
    )

    # 存储抽屉（3 条横杠）
    for y in (262, 298, 334):
        d.rounded_rectangle(
            [P(168, y)[0], P(168, y)[1], P(344, y + 16)[0], P(344, y + 16)[1]],
            radius=int(round(8 * scale)), fill=DRAWER,
        )

    # 发送角标：白色圆 + Material「send」箭头（translate(372,92) scale(2)）
    cx, cy, cr = 396, 116, 52
    d.ellipse(
        [P(cx - cr, cy - cr)[0], P(cx - cr, cy - cr)[1], P(cx + cr, cy + cr)[0], P(cx + cr, cy + cr)[1]],
        fill=WHITE,
    )
    send = [(2.01, 21), (23, 12), (2.01, 3), (2, 10), (17, 12), (2, 14)]
    d.polygon([P(372 + 2 * x, 92 + 2 * y) for x, y in send], fill=ARROW)

    return img.resize((size, size), Image.LANCZOS)


def main():
    sizes = [16, 32, 48, 256]
    big = draw_icon(sizes[-1])
    small = [draw_icon(s) for s in sizes[:-1]]
    big.save(
        "icon.ico", format="ICO",
        sizes=[(s, s) for s in sizes], append_images=small,
    )
    # 同时导出 256px 预览图，便于肉眼确认圆角效果
    big.save("icon_preview.png")
    print("generated icon.ico (16/32/48/256) + icon_preview.png")


if __name__ == "__main__":
    main()
