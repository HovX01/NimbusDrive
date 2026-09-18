# Nimbus Social Connector

This is a local Chrome/Edge extension for Nimbus. It lets the Settings > Connect button open TikTok, Instagram, or Facebook, wait for your normal browser login session, and send a cookies.txt session back to Nimbus.

Install once:

1. Open `chrome://extensions` or `edge://extensions`.
2. Enable Developer mode.
3. Choose Load unpacked.
4. Select this folder: `web/public/social-connector`.

The extension only reads TikTok, Instagram, and Facebook cookies after you click Connect in Nimbus. It is scoped to Nimbus on `localhost`, `127.0.0.1`, and `192.168.100.49`.
