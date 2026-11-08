import { createApp } from 'vue'
import { createPinia } from 'pinia'
import PrimeVue from 'primevue/config'
import ConfirmationService from 'primevue/confirmationservice'
import ToastService from 'primevue/toastservice'
import 'primeicons/primeicons.css'

import App from './App.vue'
import { router } from './router'
import { initTheme } from './theme/theme'
import { KapojPreset } from './theme/preset'
import { primeVueLocale } from './theme/primevue-locale'
import './styles/tokens.css'
import './styles/base.css'
import './styles.css'

initTheme()

const app = createApp(App)
app.use(createPinia())
app.use(router)
app.use(PrimeVue, {
  ripple: false,
  inputVariant: 'outlined',
  locale: primeVueLocale,
  theme: {
    preset: KapojPreset,
    options: {
      darkModeSelector: '.app-dark',
      cssLayer: false,
    },
  },
})
app.use(ToastService)
app.use(ConfirmationService)
app.mount('#app')
