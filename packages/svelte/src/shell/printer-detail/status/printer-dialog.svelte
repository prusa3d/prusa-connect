<!--
  The question on the printer's own screen, with one button per answer the
  firmware offers. Nothing is drawn while the printer waits on none.

  The labels are the words on the printer's own screen, from the catalogue, so
  they follow the language. The firmware's name of the button is the answer the
  printer is sent, and it labels a button the catalogue does not hold.

  A firmware may send the buttons and no words at all, which is what a filament
  change does: the question stands on the machine's own screen and reaches the
  tree as a code. Buttons under a bare heading ask somebody who is not at the
  machine to answer a question they cannot read. The catalogue names the code
  in the language on screen, as Prusa-Error-Codes does. For a code it does not
  hold, the banner says the words did not arrive and gives the number, which is
  the one thing that says which question it is.

  What the printer said itself is kept: the firmware builds some of its texts
  as it goes, and those are more exact than the catalogue's. For such a code
  the catalogue holds the title and no text, and the banner then says the
  question is on the machine, under the title.
-->
<script lang="ts">
  import MessageCircleQuestionIcon from '@lucide/svelte/icons/message-circle-question-mark';

  import {
    dialogAnswer,
    dialogButtonLabel,
    dialogOf,
    projectCommand,
    type Device,
  } from '../../../domain';
  import { describePrinterCode, printerButtonLabel, t } from '../../../i18n';
  import {
    Alert,
    AlertDescription,
    AlertTitle,
    Button,
  } from '../../../primitives';
  import { refusalText } from '../controls/machine-controls';
  import { mayOperate } from '../info/permission';
  import type { CommandPort, TelemetrySource } from '../types';
  import { wireStateOf } from './presentation';

  let {
    device,
    telemetry,
    commands,
  }: {
    device: Device;
    telemetry: TelemetrySource;
    commands: CommandPort;
  } = $props();

  const VERB = 'core.command.dialog_action';

  // The codes and the button names in the catalogue are the Buddy firmware's.
  // Another driver's code of the same digits is another question.
  const BUDDY_DRIVER = 'xbuddy';
  const buddy = $derived(device.driver === BUDDY_DRIVER);

  // A printer that is away is waiting on nothing anybody here can answer.
  const dialog = $derived(device.connected ? dialogOf(device) : undefined);

  const projection = $derived(
    projectCommand(VERB, {
      capabilities: device.capabilities,
      transitions: commands.transitions ?? [],
      state: wireStateOf(telemetry, device),
      locked: commands.locked ?? false,
      permitted: device.verbs,
    }),
  );

  const permitted = $derived(mayOperate(device));

  // A model that cannot be answered remotely is answered at the machine.
  const answerable = $derived(
    projection.kind !== 'absent' && (dialog?.buttons.length ?? 0) > 0,
  );

  const reason = $derived.by(() => {
    if (!permitted) return t('printerDetailStatus.dialog.notPermitted');

    return projection.kind === 'disabled'
      ? refusalText(projection.reason)
      : undefined;
  });

  const named = $derived(buddy ? describePrinterCode(dialog?.code) : undefined);
  const title = $derived(dialog?.title ?? named?.title);
  const text = $derived(dialog?.text ?? named?.text);

  // Buttons to press and no words saying what they answer. Said as its own
  // sentence rather than left as an empty space: an empty space reads as
  // "there is nothing more to say", and the sentence says the words are
  // missing and where to read them.
  //
  // It needs the answers as well as the missing words. A dialog that reached
  // this server as an id alone, which is what telemetry carries between the
  // events that describe one, has no buttons either, and there the sentence
  // would promise answers that are not on the screen and point at a press
  // nobody can make. That case already has its own line below.
  //
  // A title the catalogue gave is not the question: it names the code and the
  // sentence the firmware fills in stays on the machine. A title the printer
  // sent is what the printer chose to say, and stands alone.
  const wordless = $derived(
    answerable && dialog?.title === undefined && text === undefined,
  );
</script>

{#if dialog}
  <Alert variant="warning">
    <MessageCircleQuestionIcon aria-hidden="true" />
    <AlertTitle>
      {title ?? t('printerDetailStatus.dialog.heading')}
    </AlertTitle>
    <AlertDescription class="flex flex-col gap-3">
      {#if text}
        <!-- Some codes have the title for a text, and it is said once. -->
        {#if text !== title}
          <p class="whitespace-pre-line">{text}</p>
        {/if}
      {:else if wordless}
        <p>
          {dialog.code === undefined
            ? t('printerDetailStatus.dialog.unsaid')
            : t('printerDetailStatus.dialog.unsaidCode', { code: dialog.code })}
        </p>
      {/if}
      {#if answerable}
        <div class="flex flex-wrap gap-2">
          {#each dialog.buttons as button (button)}
            <Button
              size="sm"
              variant="secondary"
              disabled={reason !== undefined || commands.sending}
              pending={commands.pending === VERB}
              onclick={() => {
                void commands.onCommand(VERB, dialogAnswer(dialog, button));
              }}
            >
              {(buddy ? printerButtonLabel(button) : undefined) ??
                dialogButtonLabel(button)}
            </Button>
          {/each}
        </div>
        {#if reason !== undefined}
          <p class="text-muted-foreground">{reason}</p>
        {/if}
      {:else}
        <p class="text-muted-foreground">
          {t('printerDetailStatus.dialog.atPrinter')}
        </p>
      {/if}
    </AlertDescription>
  </Alert>
{/if}
