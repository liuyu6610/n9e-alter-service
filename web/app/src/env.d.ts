declare module '*.vue' {
	import type { DefineComponent } from 'vue'
	const component: DefineComponent<{}, {}, any>
	export default component
}

interface ImportMetaEnv {
	readonly [key: string]: string | boolean | undefined
}

interface ImportMeta {
	readonly env: ImportMetaEnv
}
