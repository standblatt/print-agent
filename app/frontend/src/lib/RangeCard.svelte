<script lang="ts">
	import { Check, CircleCheck, CircleX, LoaderCircle, TriangleAlert, X } from '@lucide/svelte';
	import { t } from 'lingui-for-svelte/macro';
	import type { RangeState } from '$bindings/app';
	import { i18n } from '$lib/locale';

	let { range }: { range: RangeState } = $props();
	const status = $derived(range.status);
	const missing = $derived((status.info.missingPrinters ?? []).join(', '));
	const jobs = $derived((status.jobs ?? []).slice(0, 5));
	/** Date and time of a job, short, in the language of the app. */
	const when = (iso: string) => new Date(iso).toLocaleString(i18n.locale, { dateStyle: 'short', timeStyle: 'short' });
</script>

<section class="rounded-lg border bg-card p-4">
	<div class="flex items-start gap-3">
		{#if range.health === 'ok'}
			<CircleCheck class="size-8 shrink-0 text-green-600" />
		{:else if range.health === 'missing'}
			<TriangleAlert class="size-8 shrink-0 text-amber-500" />
		{:else if range.health === 'starting'}
			<LoaderCircle class="size-8 shrink-0 animate-spin text-muted-foreground" />
		{:else}
			<CircleX class="size-8 shrink-0 text-destructive" />
		{/if}
		<div class="min-w-0">
			<p class="font-medium">{range.club}, {range.range}</p>
			<p class="text-sm">
				{#if range.health === 'ok'}
					{$t`Connected. The documents of this range print here.`}
				{:else if range.health === 'missing'}
					{$t`Connected, but these printers of the printing setup are not on this computer: ${missing}.`}
				{:else if range.health === 'starting'}
					{$t`Connecting to Standblatt…`}
				{:else if range.health === 'removed'}
					{$t`This computer was removed from the range in Standblatt. Remove the range under Advanced and connect it again.`}
				{:else}
					{$t`No connection to Standblatt. Check the internet connection; the app keeps trying.`}
				{/if}
			</p>
			{#if status.err && range.health !== 'removed'}
				<p class="mt-1 text-xs break-words text-muted-foreground">{status.err}</p>
			{/if}
		</div>
	</div>
	{#if jobs.length}
		<ul class="mt-3 grid gap-1 border-t pt-3 text-sm">
			{#each jobs as job (job.id)}
				<li class="flex items-baseline gap-2">
					{#if job.err}
						<X class="size-4 shrink-0 self-center text-destructive" />
					{:else}
						<Check class="size-4 shrink-0 self-center text-green-600" />
					{/if}
					<span class="min-w-0 truncate">{job.title}</span>
					<span class="ml-auto shrink-0 text-xs text-muted-foreground">{job.printer} · {when(job.at)}</span>
				</li>
				{#if job.err}<li class="pl-6 text-xs text-destructive">{job.err}</li>{/if}
			{/each}
		</ul>
	{/if}
</section>
