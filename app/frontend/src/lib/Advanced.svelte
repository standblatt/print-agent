<script lang="ts">
	import { t } from 'lingui-for-svelte/macro';
	import { onMount } from 'svelte';
	import { Service, type RangeState, type Settings } from '$bindings/app';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Switch } from '$lib/components/ui/switch';
	import { message } from '$lib/message';

	let { ranges, refresh }: { ranges: RangeState[]; refresh: () => Promise<void> } = $props();
	let settings = $state<Settings | null>(null);
	let server = $state('');
	let saved = $state(false);
	let error = $state('');
	/** The range whose Remove was clicked once. */
	let removing = $state('');

	/** Loads the settings from the Go side. */
	async function load() {
		settings = await Service.Settings();
		server = settings.server;
	}

	/** Saves the address of Standblatt for new ranges. */
	async function saveServer(e: SubmitEvent) {
		e.preventDefault();
		error = '';
		saved = false;
		try {
			await Service.SetServer(server);
			saved = true;
			await load();
		} catch (err) {
			error = message(err);
		}
	}

	/** Turns starting with the computer on or off. */
	async function setAutostart(on: boolean) {
		error = '';
		try {
			await Service.SetAutostart(on);
		} catch (err) {
			error = message(err);
		}
		await load();
	}

	/** Removes a range on the second click. */
	async function remove(id: string) {
		if (removing !== id) {
			removing = id;
			return;
		}
		removing = '';
		await Service.Remove(id);
		await refresh();
	}
	onMount(load);
</script>

<details class="rounded-lg border bg-card p-4">
	<summary class="cursor-pointer font-medium">{$t`Advanced`}</summary>
	<div class="mt-4 grid gap-5">
		<label class="flex items-center justify-between gap-4">
			<span class="text-sm">
				{$t`Start with the computer`}
				<span class="block text-xs text-muted-foreground">{$t`Recommended: jobs print as soon as the computer is on.`}</span>
			</span>
			<Switch checked={settings?.autostart ?? false} onCheckedChange={setAutostart} />
		</label>
		<form class="grid gap-1.5" onsubmit={saveServer}>
			<label for="server" class="text-sm">{$t`Address of Standblatt`}</label>
			<div class="flex gap-2">
				<Input id="server" bind:value={server} oninput={() => (saved = false)} />
				<Button type="submit" variant="outline">{$t`Save`}</Button>
			</div>
			<p class="text-xs text-muted-foreground">
				{#if saved}{$t`Saved.`}{/if}
				{$t`Only change it for a test server; connected ranges keep their address.`}
			</p>
		</form>
		{#if error}<p class="text-sm text-destructive">{error}</p>{/if}
		{#if ranges.length}
			<div>
				<p class="text-sm">{$t`Ranges`}</p>
				<ul class="mt-1 divide-y">
					{#each ranges as r (r.id)}
						<li class="flex items-center justify-between gap-2 py-1.5 text-sm">
							<span class="min-w-0 truncate">{r.club}, {r.range}</span>
							<Button variant="destructive" size="sm" onclick={() => remove(r.id)}>
								{removing === r.id ? $t`Really remove?` : $t`Remove`}
							</Button>
						</li>
					{/each}
				</ul>
				<p class="mt-1 text-xs text-muted-foreground">
					{$t`Removing stops printing on this computer. Delete the print agent in Standblatt as well.`}
				</p>
			</div>
		{/if}
		<p class="text-xs text-muted-foreground">{$t`Version ${settings?.version ?? ''}`}</p>
	</div>
</details>
