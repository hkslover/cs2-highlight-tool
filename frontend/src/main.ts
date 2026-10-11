import { createApp } from 'vue'
import App from './App.vue'
import router from './app/router'
import './app/style.css';

// Prevent browser native drag-and-drop of text, images, and links across the app
window.addEventListener('dragstart', (event) => {
  event.preventDefault();
});

createApp(App).use(router).mount('#app')
