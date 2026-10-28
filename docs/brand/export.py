# /// script
# requires-python = ">=3.10"
# dependencies = ["playwright==1.60.0"]
# ///
"""Export clau's social preview, share animation, and logo PNGs.

    uv run docs/brand/export.py

Writes next to this file:

    social-preview.png  1280x640 still, for GitHub's Settings > Social preview
    board.mp4           the flip animation, for X, LinkedIn and the like
    board.gif           the same animation as a looping GIF
    logo-<n>.png        logo.svg at 512, 256, 64, 32 and 16 pixels

Needs ffmpeg on PATH and Playwright's Chromium. If the browser is
missing, install it once with:

    uv run --with playwright==1.60.0 playwright install chromium

board.html loads its font from Google Fonts, so exporting needs a
network connection.
"""

import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

from playwright.sync_api import sync_playwright

HERE = Path(__file__).resolve().parent
HOLD = 3.5  # seconds the settled board stays up before the loop restarts
LOGO_SIZES = (512, 256, 64, 32, 16)


def ffmpeg(*args):
    subprocess.run(["ffmpeg", "-y", "-loglevel", "error", *map(str, args)], check=True)


def export_board(browser, tmp):
    page = browser.new_page(viewport={"width": 1280, "height": 640})
    page.goto((HERE / "board.html").as_uri() + "#export")
    page.wait_for_selector("body[data-ready]")
    board = page.locator("#board")
    duration, tick = page.evaluate("[clauBoard.duration, clauBoard.tick]")

    page.evaluate("t => clauBoard.render(t)", duration)
    board.screenshot(path=HERE / "social-preview.png")

    # Letters only change on tick boundaries, so one frame per tick
    # captures the flip exactly; the last frame is the settled board,
    # which tpad then holds.
    steps = int(duration // tick) + 1
    for i in range(steps + 1):
        page.evaluate("t => clauBoard.render(t)", i * tick)
        board.screenshot(path=tmp / f"{i:04d}.png")

    frames = ["-framerate", f"1000/{tick}", "-i", tmp / "%04d.png"]
    hold = f"tpad=stop_mode=clone:stop_duration={HOLD}"
    ffmpeg(*frames, "-vf", f"{hold},fps=30,format=yuv420p", "-c:v", "libx264",
           "-crf", "18", "-movflags", "+faststart", HERE / "board.mp4")
    ffmpeg(*frames, "-vf",
           f"{hold},split[a][b];[a]palettegen=stats_mode=diff[p];[b][p]paletteuse=dither=none",
           "-loop", "0", HERE / "board.gif")


def export_logo(browser):
    svg = (HERE / "logo.svg").read_text()
    page = browser.new_page(viewport={"width": 600, "height": 600})
    for n in LOGO_SIZES:
        page.set_content(
            "<style>html,body{margin:0;background:transparent}"
            f"svg{{display:block;width:{n}px;height:{n}px}}</style>{svg}")
        page.locator("svg").screenshot(path=HERE / f"logo-{n}.png", omit_background=True)


def main():
    if not shutil.which("ffmpeg"):
        sys.exit("export.py: ffmpeg not found on PATH")
    with sync_playwright() as p, tempfile.TemporaryDirectory() as tmp:
        browser = p.chromium.launch()
        export_board(browser, Path(tmp))
        export_logo(browser)
        browser.close()


if __name__ == "__main__":
    main()
