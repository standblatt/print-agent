<script lang="ts">
	import { t } from 'lingui-for-svelte/macro';
	import { Service, type UpdateState } from '$bindings/app';
	import { Button } from '$lib/components/ui/button';
	import { message } from '$lib/message';

	let { update }: { update: UpdateState } = $props();
	let busy = $state(false);
	let error = $state('');
	// Installing from here or from the tray; the app restarts once it is done.
	const installing = $derived(busy || (!!update.state && !update.err));

	/** Downloads and checks the newer release; the app then restarts with it. */
	async function install() {
		busy = true;
		error = '';
		try {
			await Service.InstallUpdate();
		} catch (e) {
			error = message(e);
		} finally {
			busy = false;
		}
	}
</script>

<section class="flex flex-wrap items-center gap-3 rounded-lg border border-amber-500/50 bg-amber-50 p-4 dark:bg-amber-950/30">
	<p class="min-w-0 flex-1 text-sm">
		{#if installing}
			{$t`Updating to version ${update.version}… The app restarts by itself.`}
		{:else}
			{$t`Version ${update.version} is available.`}
		{/if}
		{#if update.err || error}
			<span class="mt-1 block text-xs break-words text-destructive">{update.err || error}</span>
		{/if}
	</p>
	<Button onclick={install} disabled={installing}>{$t`Update now`}</Button>
</section>
