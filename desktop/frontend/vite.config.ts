import { defineConfig, type Plugin } from "vite";
import vue from "@vitejs/plugin-vue";
import tailwindcss from "@tailwindcss/vite";
import wails from "@wailsio/runtime/plugins/vite";

// Strict Content-Security-Policy for production builds: only local assets,
// no remote origins, no eval. (Dev builds skip it so Vite's HMR works.)
function csp(): Plugin {
  return {
    name: "clawdyx-csp",
    apply: "build",
    transformIndexHtml(html) {
      const policy = [
        "default-src 'self'",
        "script-src 'self'",
        "style-src 'self' 'unsafe-inline'",
        "img-src 'self' data: blob:",
        "font-src 'self'",
        "connect-src 'self'",
        "object-src 'none'",
        "base-uri 'none'",
        "form-action 'none'",
        "frame-ancestors 'none'",
      ].join("; ");
      return html.replace("<head>", `<head>\n    <meta http-equiv="Content-Security-Policy" content="${policy}">`);
    },
  };
}

export default defineConfig({
  server: { host: "127.0.0.1", port: Number(process.env.WAILS_VITE_PORT) || 9245, strictPort: true },
  plugins: [vue(), tailwindcss(), wails("./bindings"), csp()],
  build: { target: "safari16", sourcemap: false, chunkSizeWarningLimit: 600 },
  test: { environment: "happy-dom" },
} as any);
