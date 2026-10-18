# Screens and camera

Both are off until you turn them on in **Settings → Features**. Nothing is ever captured at startup or on a timer, and every capture is recorded in the audit table (what, how large, where it went; never the pixels).

## What works here

{{if .Darwin -}}
Screenshots and Claude's mouse and keyboard tools work once macOS allows them: Screen Recording for screenshots, Accessibility for the mouse and keyboard (see [Permissions](help:permissions)).
{{- else if .Windows -}}
Screenshots and Claude's mouse and keyboard tools work without extra permissions. Two limits come from Windows itself: input cannot reach windows that run as administrator, and nothing can be captured or clicked while the PC is locked or on the sign-in screen.
{{- else if eq .Session "x11" -}}
This is an X11 session: screenshots of every monitor and Claude's mouse and keyboard tools work.
{{- else if eq .Session "wayland" -}}
This is a Wayland session: screenshots work through the desktop's screenshot service (the desktop may ask on the screen the first time), and the whole screen is one display. Claude's mouse and keyboard tools are not available on Wayland yet. To use them, log in to an X11 session (on the login screen, choose the session ending in "on Xorg" or "X11").
{{- else -}}
There is no graphical session here (a server, or the worker started without a desktop), so screen features are off. The camera still works.
{{- end}}

## Screens

- `/screens` lists the displays. It never captures.
- `/screen 2` sends you a photo of display 2 (1 is the main display); `/screen all` sends one per display. Photos go only to the private chat that asked.
- `/look 2 what is wrong here?` lets Claude look at display 2 in the active session.
- In automation sessions Claude also gets screen tools: screenshot, click, move the mouse, scroll, type and press keys. Before each screenshot the tool asks the worker whether screen features are still on.
- **Monitors** in this app shows every display and captures one only when you click.

Captures stay in memory for at most five minutes.

## Camera

{{if .Darwin -}}
- `/cameras` lists the cameras (built-in first) without capturing; `/camera` sends a photo; `/camera video 10` sends a 10-second clip without sound (1–60 seconds).
- macOS asks once, on the Mac, at the first `/camera`. The green light is on while the camera records.
{{- else if .Windows -}}
- `/cameras` lists the cameras without capturing; `/camera` sends a photo; `/camera video 10` sends a 10-second clip without sound (1–60 seconds).
- Camera access for desktop apps must be on: Windows Settings → Privacy & security → Camera. Windows shows the camera icon in the taskbar while it records.
- Windows N editions need the Media Feature Pack (Settings → Apps → Optional features) for clips.
{{- else -}}
- `/cameras` lists the cameras without capturing; `/camera` sends a photo; `/camera video 10` sends a 10-second clip without sound (1–60 seconds).
- Your user needs access to the camera device: be a member of the `video` group (`sudo usermod -aG video $USER`, then log in again).
- Photos need nothing else. Clips are recorded by ffmpeg (`sudo apt install ffmpeg`, `sudo dnf install ffmpeg` or `sudo pacman -S ffmpeg`).
{{- end}}
- Only the owner can use the camera, and only in their private chat. Claude never gets the camera: there is no camera tool.

## Switching everything off

`/screen off` switches every capture path off at once: Telegram, this app, Claude's screen tools and the camera. Only someone at {{.W.TheComputer}} can switch it on again, in this app.
