import babelExtractor from '@lingui/cli/api/extractors/babel';
import { formatter } from '@lingui/format-po-gettext';
import { defineConfig } from 'lingui-for-svelte/config';
import { svelteExtractor } from 'lingui-for-svelte/extractor';

export default defineConfig({
	locales: ['en', 'de', 'fr', 'it'],
	sourceLocale: 'en',
	catalogs: [{ path: 'src/locales/{locale}', include: ['src'] }],
	format: formatter({ origins: true, lineNumbers: false }),
	extractors: [svelteExtractor, babelExtractor]
});
