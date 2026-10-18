<script lang="ts">
	import { RefreshCw } from '@lucide/svelte';
	import { t } from 'lingui-for-svelte/macro';
	import { onMount } from 'svelte';
	import { Service } from '$bindings/app';
	import type { Printer } from '$bindings/internal/agent';
	import { Button } from '$lib/components/ui/button';
	import { message } from '$lib/message';

	let printers = $state<Printer[] | null>(null);
	let error = $state('');
	/** Per printer: busy while the test page prints, then ok or the error. */
	let tests = $state<Record<string, string>>({});

	/** Lists the printers of this computer. */
	async function load() {
		error = '';
		try {
			printers = (await Service.Printers()) ?? [];
		} catch (e) {
			error = message(e);
		}
	}

	/** Prints the test page on a printer. */
	async function testPage(name: string) {
		tests[name] = 'busy';
		try {
			await Service.TestPage(name);
			tests[name] = 'ok';
		} catch (e) {
			tests[name] = message(e);
		}
	}
	onMount(load);
</script>

<section class="rounded-lg border bg-card p-4">
	<div class="flex items-center justify-between gap-2">
		<h2 class="font-medium">{$t`Printers of this computer`}</h2>
		<Button variant="ghost" size="sm" onclick={load}><RefreshCw />{$t`Refresh`}</Button>
	</div>
	{#if error}
		<p class="mt-2 text-sm text-destructive">{error}</p>
	{:else if printers?.length === 0}
		<p class="mt-2 text-sm text-muted-foreground">{$t`No printers found. Install the printer on this computer first.`}</p>
	{:else if printers}
		<ul class="mt-2 divide-y">
			{#each printers as p (p.name)}
				{@const test = tests[p.name]}
				<li class="flex items-center gap-2 py-2">
					<span class="min-w-0 flex-1 truncate text-sm">{p.name}</span>
					{#if test === 'ok'}
						<span class="text-xs text-green-600">{$t`Sent to the printer`}</span>
					{:else if test && test !== 'busy'}
						<span class="max-w-1/2 truncate text-xs text-destructive" title={test}>{test}</span>
					{/if}
					<Button variant="outline" size="sm" disabled={test === 'busy'} onclick={() => testPage(p.name)}>{$t`Test page`}</Button>
				</li>
			{/each}
		</ul>
	{/if}
</section>
