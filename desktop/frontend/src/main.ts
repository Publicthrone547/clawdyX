import { createApp } from "vue";
import App from "./App.vue";
import "./style.css";
import { applyTheme } from "./prefs";

applyTheme();
createApp(App).mount("#app");
