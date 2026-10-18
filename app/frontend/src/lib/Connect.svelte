<script lang="ts">
	import { Browser } from '@wailsio/runtime';
	import { t } from 'lingui-for-svelte/macro';
	import { Service, type PairingState } from '$bindings/app';
	import { Button } from '$lib/components/ui/button';
	import { message } from '$lib/message';

	let { pairing, first, refresh }: { pairing: PairingState | null; first: boolean; refresh: () => Promise<void> } = $props();
	let error = $state('');
	let busy = $state(false);

	/** Asks the server for a code to confirm in Standblatt. */
	async function start() {
		error = '';
		busy = true;
		try {
			await Service.StartPairing();
			await refresh();
		} catch (e) {
			error = message(e);
		} finally {
			busy = false;
		}
	}

	/** Stops waiting for the confirmation. */
	async function cancel() {
		await Service.CancelPairing();
		await refresh();
	}
</script>

<section class="rounded-lg border bg-card p-4">
	{#if pairing && !pairing.err}
		<h2 class="font-medium">{$t`Connect a range`}</h2>
		<p class="mt-1 text-sm text-muted-foreground">
			{$t`Open Standblatt, log in and confirm this code. This window switches by itself as soon as you have confirmed it.`}
		</p>
		<p class="my-5 text-center font-mono text-4xl font-semibold tracking-widest select-all">{pairing.code}</p>
		<div class="flex justify-center gap-2">
			<Button onclick={() => Browser.OpenURL(pairing.url)}>{$t`Open Standblatt`}</Button>
			<Button variant="ghost" onclick={cancel}>{$t`Cancel`}</Button>
		</div>
	{:else}
		{#if first}
			<p class="mb-3 text-sm">
				{$t`This computer does not print for any range yet. Connect it with a range of your club in Standblatt; you need the permission to manage ranges there.`}
			</p>
		{/if}
		{#if pairing?.expired}
			<p class="mb-3 text-sm text-destructive">{$t`The code expired. Get a new one.`}</p>
		{:else if pairing?.err || error}
			<p class="mb-3 text-sm text-destructive">
				{$t`Standblatt cannot be reached. Check the internet connection and try again.`}
				<span class="block text-xs break-words text-muted-foreground">{pairing?.err || error}</span>
			</p>
		{/if}
		<Button onclick={start} disabled={busy}>{first ? $t`Connect a range` : $t`Connect another range`}</Button>
	{/if}
</section>
