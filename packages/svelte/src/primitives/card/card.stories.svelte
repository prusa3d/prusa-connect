<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import Badge from '../badge/badge.svelte';
  import Button from '../button/button.svelte';
  import CardAction from './card-action.svelte';
  import CardContent from './card-content.svelte';
  import CardDescription from './card-description.svelte';
  import CardFooter from './card-footer.svelte';
  import CardHeader from './card-header.svelte';
  import CardTitle from './card-title.svelte';
  import Card from './card.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/Card',
    component: Card,
  });
</script>

<Story name="Default" asChild>
  <Card class="w-80">
    <CardHeader>
      <CardTitle>
        <h3 class="contents">MK4S</h3>
      </CardTitle>
      <CardDescription>Ready to print</CardDescription>
      <CardAction>
        <Badge variant="success">Online</Badge>
      </CardAction>
    </CardHeader>
    <CardContent>
      <p class="text-sm text-muted-foreground">
        Last job finished 12 minutes ago.
      </p>
    </CardContent>
    <CardFooter class="gap-2">
      <Button size="sm">Print</Button>
      <Button size="sm" variant="secondary">Details</Button>
    </CardFooter>
  </Card>
</Story>

<!-- CardTitle renders a div; class="contents" on the nested heading keeps it
     out of CardHeader's grid sizing. -->
<Story name="Heading Level Nesting" asChild>
  <div class="flex flex-wrap gap-4">
    <Card class="w-64">
      <CardHeader>
        <CardTitle>
          <h2 class="contents">MK4S</h2>
        </CardTitle>
        <CardDescription>Ready to print</CardDescription>
      </CardHeader>
      <CardContent>
        <p class="text-sm text-muted-foreground">Idle</p>
      </CardContent>
    </Card>
    <Card class="w-64">
      <CardHeader>
        <CardTitle>
          <h2 class="contents">XL</h2>
        </CardTitle>
        <CardDescription>Printing</CardDescription>
      </CardHeader>
      <CardContent>
        <p class="text-sm text-muted-foreground">42% complete</p>
      </CardContent>
    </Card>
  </div>
</Story>

<Story name="Landmark Region" asChild>
  <Card class="w-80" role="region" aria-labelledby="landmark-region-card-title">
    <CardHeader>
      <CardTitle id="landmark-region-card-title">
        <h2 class="contents">MK4S</h2>
      </CardTitle>
      <CardDescription>Ready to print</CardDescription>
    </CardHeader>
    <CardContent>
      <p class="text-sm text-muted-foreground">
        Last job finished 12 minutes ago.
      </p>
    </CardContent>
  </Card>
</Story>

<Story name="Densities" asChild>
  <div class="grid gap-4 @md:grid-cols-4">
    <Card density="default"><CardContent>Default</CardContent></Card>
    <Card density="compact"><CardContent>Compact</CardContent></Card>
    <Card density="tight"><CardContent>Tight</CardContent></Card>
    <Card density="flush"><CardContent>Flush</CardContent></Card>
  </div>
</Story>

<Story name="Tones" asChild>
  <div class="grid gap-4 @md:grid-cols-3">
    <Card tone="default"><CardContent>Default</CardContent></Card>
    <Card tone="attention"><CardContent>Attention</CardContent></Card>
    <Card tone="critical"><CardContent>Critical</CardContent></Card>
  </div>
</Story>

<!-- `compact` with 8px between the parts, as a printer card draws. -->
<Story name="Snug" asChild>
  <Card class="w-80" density="snug">
    <CardHeader><CardTitle>Original Prusa MK4S</CardTitle></CardHeader>
    <CardContent>Ready to print</CardContent>
  </Card>
</Story>

<!-- A card standing inside a panel: a fill and no border. -->
<Story name="Surface" asChild>
  <Card class="w-80" tone="surface">
    <CardContent>PrusaConnect: 3 printers</CardContent>
  </Card>
</Story>

<!-- A page's own panel: the content carries the block padding. -->
<Story name="Panel" asChild>
  <Card class="w-80" density="panel" radius="md">
    <CardContent class="py-6">Ready to print</CardContent>
  </Card>
</Story>
