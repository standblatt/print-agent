import { svelte } from '@sveltejs/vite-plugin-svelte';
import tailwindcss from '@tailwindcss/vite';
import linguiForSvelte from 'lingui-for-svelte/unplugin/vite';
import linguiMacro from 'unplugin-lingui-macro/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	server: { host: '127.0.0.1', port: Number(process.env.WAILS_VITE_PORT) || 9245, strictPort: true },
	resolve: { alias: { $lib: '/src/lib', $locales: '/src/locales' } },
	plugins: [linguiMacro(), linguiForSvelte(), tailwindcss(), svelte()]
});
