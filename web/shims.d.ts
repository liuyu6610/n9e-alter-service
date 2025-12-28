// 这些声明用于在未执行 npm install 时减少 IDE 类型报错。
// 实际构建/运行请在 web 目录执行 npm install。

declare module 'vite' {
	export type UserConfigExport = any
	export function defineConfig(config: any): any
}

declare module '@vitejs/plugin-vue' {
	const plugin: any
	export default plugin
}

declare module 'vue'
declare module 'vue-router'
declare module 'element-plus'
declare module '@element-plus/icons-vue'
