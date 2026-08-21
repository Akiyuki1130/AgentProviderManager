# Regenerates the app icon (white tile + light-blue download glyph) into
# PNG/ICO. Source of truth is assets/app-icon.svg.
from PIL import Image, ImageDraw

BASE = 1024
SS = 4  # supersample factor
S = BASE * SS

img = Image.new("RGBA", (S, S), (0, 0, 0, 0))
d = ImageDraw.Draw(img)


def rr(x0, y0, x1, y1, r, fill=None, outline=None, width=1):
    d.rounded_rectangle([x0, y0, x1, y1], radius=r, fill=fill, outline=outline, width=width)


# Rounded white tile with a thin light-gray border
rr(SS * 32, SS * 32, S - SS * 32, S - SS * 32, SS * 224,
   fill=(255, 255, 255, 255), outline=(230, 230, 230, 255), width=SS * 3)

# Download glyph in light blue
BLUE = (56, 169, 255, 255)

# Arrow shaft
rr(SS * 492, SS * 232, SS * 532, SS * 520, SS * 44, fill=BLUE)
# Arrowhead chevron
d.polygon([(SS * 364, SS * 408), (SS * 660, SS * 408), (SS * 512, SS * 572)], fill=BLUE)
# Tray base
rr(SS * 224, SS * 672, S - SS * 224, SS * 760, SS * 44, fill=BLUE)

# Downscale with smooth antialiasing
img = img.resize((BASE, BASE), Image.LANCZOS)

import os

root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
img.save(os.path.join(root, "build", "appicon.png"))
img.save(os.path.join(root, "assets", "app-icon.png"))

# ICO with common sizes
sizes = [(16, 16), (24, 24), (32, 32), (48, 48), (64, 64), (128, 128), (256, 256)]
frames = [img.resize((w, h), Image.LANCZOS) for w, h in sizes]
img.save(os.path.join(root, "assets", "app-icon.ico"), sizes=sizes, append_images=frames)
img.save(os.path.join(root, "build", "windows", "icon.ico"), sizes=sizes, append_images=frames)

print("icon done")
