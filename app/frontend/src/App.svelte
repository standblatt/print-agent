<script lang="ts">
	import { setLinguiContext } from 'lingui-for-svelte';
	import { t } from 'lingui-for-svelte/macro';
	import { onMount } from 'svelte';
	import { Service, type State } from '$bindings/app';
	import Advanced from '$lib/Advanced.svelte';
	import Connect from '$lib/Connect.svelte';
	import { i18n } from '$lib/locale';
	import Logo from '$lib/Logo.svelte';
	import Printers from '$lib/Printers.svelte';
	import RangeCard from '$lib/RangeCard.svelte';
	import UpdateBanner from '$lib/UpdateBanner.svelte';

	setLinguiContext(i18n);
	let state = $state<State>({ ranges: [], pairing: null, update: null });
	// Stays in the tray label; the Go side puts the version in.
	const version = '{version}';
	const ranges = $derived(state.ranges ?? []);

	/** Fetches the ranges and the pairing from the Go side. */
	async function refresh() {
		state = await Service.State();
	}
	onMount(() => {
		Service.SetMenuLabels($t`Open`, $t`Quit`, $t`Update to ${version}`);
		refresh();
		const timer = setInterval(refresh, 2000);
		return () => clearInterval(timer);
	});
</script>

<main class="mx-auto grid max-w-2xl gap-4 p-6">
	<header class="flex items-center gap-3">
		<Logo class="size-8 text-brand-900 dark:text-amber-400" />
		<h1 class="text-xl font-semibold tracking-tight">Standblatt Print Agent</h1>
	</header>
	{#if state.update}
		<UpdateBanner update={state.update} />
	{/if}
	{#each ranges as range (range.id)}
		<RangeCard {range} />
	{/each}
	<Connect pairing={state.pairing} first={ranges.length === 0} {refresh} />
	<Printers />
	<Advanced {ranges} {refresh} />
</main>
