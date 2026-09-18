import { defineConfig } from "vite";

export default defineConfig({
  server: {
    port: 5199,
    proxy: {
      "/api": {
        target: "http://localhost:9090",
        changeOrigin: true,
      },
    },
  },
});
