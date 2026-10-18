<script lang="ts">
	import { setLinguiContext } from 'lingui-for-svelte';
	import { t } from 'lingui-for-svelte/macro';
	import { i18n } from '$lib/locale';

	setLinguiContext(i18n);
</script>

<main class="mx-auto max-w-2xl p-6">
	<h1 class="text-2xl font-semibold tracking-tight text-brand-900 dark:text-foreground">Standblatt Print Agent</h1>
	<p class="mt-2 text-muted-foreground">{$t`Prints the documents of your range on the printers of this computer.`}</p>
</main>
