<script lang="ts">
  import { onDestroy } from 'svelte';
  import { t } from '../../i18n';
  import {
    Button,
    Dialog,
    DialogContent,
    DialogHeader,
    DialogTitle,
    DialogDescription,
    DialogFooter,
    Slider,
  } from '../../primitives';
  import { avatarCrop, AVATAR_PIXELS } from './avatar-crop';

  let {
    file,
    onConfirm,
    onCancel,
  }: { file: File; onConfirm: (file: File) => void; onCancel: () => void } =
    $props();
  let image = $state.raw<HTMLImageElement>();
  let canvas: HTMLCanvasElement | undefined = $state();
  let failed = $state(false);
  let saving = $state(false);
  let zoom = $state(1);
  let x = $state(0);
  let y = $state(0);
  let active = true;
  let drag: { id: number; x: number; y: number } | undefined;
  const clamp = (value: number): number => Math.min(1, Math.max(-1, value));

  onDestroy(() => {
    active = false;
  });
  $effect(() => {
    const url = URL.createObjectURL(file);
    const next = new Image();
    next.onload = () => {
      image = next;
    };
    next.onerror = () => {
      failed = true;
    };
    next.src = url;
    return () => {
      next.onload = null;
      next.onerror = null;
      URL.revokeObjectURL(url);
    };
  });

  $effect(() => {
    if (canvas === undefined || image === undefined) return;
    const ctx = canvas.getContext('2d');
    if (ctx === null) return;
    const crop = avatarCrop(
      image.naturalWidth,
      image.naturalHeight,
      zoom,
      x,
      y,
    );
    ctx.clearRect(0, 0, AVATAR_PIXELS, AVATAR_PIXELS);
    ctx.save();
    ctx.beginPath();
    ctx.arc(
      AVATAR_PIXELS / 2,
      AVATAR_PIXELS / 2,
      AVATAR_PIXELS / 2,
      0,
      Math.PI * 2,
    );
    ctx.clip();
    ctx.drawImage(
      image,
      crop.x,
      crop.y,
      crop.side,
      crop.side,
      0,
      0,
      AVATAR_PIXELS,
      AVATAR_PIXELS,
    );
    ctx.restore();
  });

  const move = (
    event: PointerEvent & { currentTarget: HTMLButtonElement },
  ): void => {
    if (image === undefined || drag?.id !== event.pointerId) return;
    // Pointer deltas must be mapped from the rendered canvas to source pixels.
    const box = event.currentTarget.getBoundingClientRect();
    const crop = avatarCrop(
      image.naturalWidth,
      image.naturalHeight,
      zoom,
      x,
      y,
    );
    const dx = ((event.clientX - drag.x) * crop.side) / box.width;
    const dy = ((event.clientY - drag.y) * crop.side) / box.height;
    if (image.naturalWidth > crop.side)
      x = clamp(x + (2 * dx) / (image.naturalWidth - crop.side));
    if (image.naturalHeight > crop.side)
      y = clamp(y + (2 * dy) / (image.naturalHeight - crop.side));
    drag = { id: event.pointerId, x: event.clientX, y: event.clientY };
  };
  const changeZoom = (value: number): void => {
    zoom = value;
    if (image === undefined) return;
    const crop = avatarCrop(
      image.naturalWidth,
      image.naturalHeight,
      zoom,
      x,
      y,
    );
    if (image.naturalWidth <= crop.side) x = 0;
    if (image.naturalHeight <= crop.side) y = 0;
  };
  const keyMove = (event: KeyboardEvent): void => {
    if (
      image === undefined ||
      !['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown'].includes(event.key)
    )
      return;
    event.preventDefault();
    const crop = avatarCrop(
      image.naturalWidth,
      image.naturalHeight,
      zoom,
      x,
      y,
    );
    if (image.naturalWidth > crop.side) {
      if (event.key === 'ArrowLeft') x = clamp(x - 0.05);
      if (event.key === 'ArrowRight') x = clamp(x + 0.05);
    }
    if (image.naturalHeight > crop.side) {
      if (event.key === 'ArrowUp') y = clamp(y - 0.05);
      if (event.key === 'ArrowDown') y = clamp(y + 0.05);
    }
  };
  const confirm = (): void => {
    if (image === undefined || saving) return;
    saving = true;
    const output = document.createElement('canvas');
    output.width = AVATAR_PIXELS;
    output.height = AVATAR_PIXELS;
    const ctx = output.getContext('2d');
    if (ctx === null) {
      failed = true;
      saving = false;
      return;
    }
    const crop = avatarCrop(
      image.naturalWidth,
      image.naturalHeight,
      zoom,
      x,
      y,
    );
    ctx.drawImage(
      image,
      crop.x,
      crop.y,
      crop.side,
      crop.side,
      0,
      0,
      AVATAR_PIXELS,
      AVATAR_PIXELS,
    );
    output.toBlob((blob) => {
      if (!active) return;
      if (blob === null) {
        failed = true;
        saving = false;
        return;
      }
      onConfirm(new File([blob], 'avatar.png', { type: 'image/png' }));
    }, 'image/png');
  };
</script>

<Dialog
  open
  onOpenChange={(open) => {
    if (!open) onCancel();
  }}
>
  <DialogContent class="max-h-screen overflow-y-auto">
    <DialogHeader>
      <DialogTitle>{t('settingsAccount.crop.title')}</DialogTitle>
      <DialogDescription>{t('settingsAccount.crop.help')}</DialogDescription>
    </DialogHeader>
    {#if failed}
      <!-- eslint-disable-next-line no-restricted-syntax -- the crop dialog announces the read error in place as one red line; Alert would box it -->
      <p role="alert" class="text-sm text-destructive-text">
        {t('settingsAccount.identity.photoReadError')}
      </p>
    {:else}
      <div class="flex min-w-0 justify-center">
        <Button
          size="icon-md"
          variant="ghost"
          radius="full"
          data-slot="avatar-crop-viewport"
          disabled={image === undefined || saving}
          label={t('settingsAccount.crop.move')}
          class="relative aspect-square h-auto w-full max-w-avatar-crop cursor-grab touch-none overflow-hidden border-0 bg-secondary p-0 active:cursor-grabbing"
          onpointerdown={(
            event: PointerEvent & { currentTarget: HTMLButtonElement },
          ) => {
            if (event.button !== 0) return;
            event.currentTarget.setPointerCapture(event.pointerId);
            drag = { id: event.pointerId, x: event.clientX, y: event.clientY };
          }}
          onpointermove={move}
          onpointerup={() => {
            drag = undefined;
          }}
          onpointercancel={() => {
            drag = undefined;
          }}
          onlostpointercapture={() => {
            drag = undefined;
          }}
          onkeydown={keyMove}
        >
          <canvas
            bind:this={canvas}
            width={AVATAR_PIXELS}
            height={AVATAR_PIXELS}
            aria-hidden="true"
            class="absolute inset-0 size-full rounded-full"
          ></canvas>
        </Button>
      </div>
      <div class="flex items-center gap-4">
        <span class="text-sm">{t('settingsAccount.crop.zoom')}</span>
        <Slider
          type="single"
          value={zoom}
          min={1}
          max={3}
          step={0.05}
          onValueChange={changeZoom}
          disabled={image === undefined || saving}
          aria-label={t('settingsAccount.crop.zoom')}
          class="min-w-0 flex-1"
        />
        <span class="text-sm tabular-nums">{Math.round(zoom * 100)}%</span>
      </div>
      {#if image === undefined}
        <p class="text-sm text-muted-foreground" role="status">
          {t('settingsAccount.crop.loading')}
        </p>
      {:else}
        <p class="sr-only" role="status">
          {t('settingsAccount.crop.position', {
            x: Math.round(x * 100),
            y: Math.round(y * 100),
          })}
        </p>
      {/if}
    {/if}
    <DialogFooter>
      <Button variant="secondary" onclick={onCancel}
        >{t('settingsAccount.crop.cancel')}</Button
      >
      <Button
        disabled={failed || image === undefined || saving}
        onclick={confirm}>{t('settingsAccount.crop.confirm')}</Button
      >
    </DialogFooter>
  </DialogContent>
</Dialog>
