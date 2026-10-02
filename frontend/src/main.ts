import { definePreset } from '@primeuix/themes'
import Aura from '@primeuix/themes/aura'
import PrimeVue from 'primevue/config'
import ConfirmationService from 'primevue/confirmationservice'
import ToastService from 'primevue/toastservice'
import Tooltip from 'primevue/tooltip'
import { createApp } from 'vue'
import './style.css'
import App from './App.vue'
import { router } from './router'

// Violet primary so the active states do not compete with the green success
// coloring used across the dashboard and issues.
const VioletPreset = definePreset(Aura, {
  // Square everything off: every component radius token resolves to 0.
  primitive: {
    borderRadius: {
      none: '0',
      xs: '0',
      sm: '0',
      md: '0',
      lg: '0',
      xl: '0',
    },
  },
  semantic: {
    primary: {
      50: '{violet.50}',
      100: '{violet.100}',
      200: '{violet.200}',
      300: '{violet.300}',
      400: '{violet.400}',
      500: '{violet.500}',
      600: '{violet.600}',
      700: '{violet.700}',
      800: '{violet.800}',
      900: '{violet.900}',
      950: '{violet.950}',
    },
  },
  components: {
    // Distinct table header and paginator footer, set through the theme so
    // PrimeVue's hover, sorted and filter states stay consistent.
    datatable: {
      headerCell: {
        background: '{surface.50}',
        color: '{surface.600}',
        borderColor: '{surface.200}',
      },
      paginatorBottom: {
        borderColor: '{surface.200}',
        borderWidth: '1px',
      },
    },
    paginator: {
      root: {
        background: '{surface.50}',
        color: '{surface.600}',
      },
    },
  },
})

const app = createApp(App)

app.use(PrimeVue, {
  theme: {
    preset: VioletPreset,
  },
  license: 'eyJpZCI6IjgxODMxNzVhLWI0NjUtNDQzYi1hMjgwLWU2ZTQzZjQ2NTE3NiIsInByb2R1Y3QiOiJwcmltZXVpIiwidGllciI6ImNvbW11bml0eSIsInR5cGUiOiJkZXYiLCJpYXQiOjE3OTA4NzUyMjQsImV4cCI6MTgyMjQxMTIyNH0.1RtAOl9Ww3-aE0yEH5e5r9qh7GVP-t1-eEaCcSpPRZUYbgszfwmC4gR5Om7J7Qd362yctgPQEgfTcLNnd6QPCA'
})
app.use(router)
app.use(ToastService)
app.use(ConfirmationService)
app.directive('tooltip', Tooltip)

// Wait for the initial route to resolve before mounting, otherwise the shell
// mounts with empty params and redirects deep links to the dashboard.
router.isReady().then(() => app.mount('#app'))
