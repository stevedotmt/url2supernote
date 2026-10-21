# EPUB from reader-mode extraction, no headless browser

Articles are extracted with Readability-style reader-mode parsing over plain HTTP and packaged as EPUB. We rejected rendering pages in a headless browser and producing device-sized PDFs: Chromium rules out the cheapest hosts and inflates cold starts, while EPUB reflows on every Supernote model. The cost is that paywalled and JavaScript-only pages fail, which we report to the Owner rather than delivering a broken file. PDF may come later as an opt-in format.
