import type { Theme } from 'vitepress'
import DefaultTheme from 'vitepress/theme'
import QuickStartBuilder from './components/QuickStartBuilder.vue'
import VerifyLevelMatrix from './components/VerifyLevelMatrix.vue'
import './style.css'

export default {
  extends: DefaultTheme,
  enhanceApp({ app }) {
    app.component('QuickStartBuilder', QuickStartBuilder)
    app.component('VerifyLevelMatrix', VerifyLevelMatrix)
  }
} satisfies Theme
