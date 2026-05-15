import { defineConfig } from "deepsec/config";

export default defineConfig({
  projects: [
    { id: "deepsec-scan", root: ".." },
    // <deepsec:projects-insert-above>
  ],
});
