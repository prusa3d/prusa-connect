import bracket from './assets/job-bracket.png';
import clip from './assets/job-clip.png';
import spacer from './assets/job-spacer.png';
import gearbox from './assets/printer-gearbox-housing.png';
import phoneStand from './assets/printer-phone-stand.png';
import spoolHolder from './assets/printer-spool-holder.png';

const previews = new Map([
  ['usb/examples/bracket.bgcode', bracket],
  ['usb/examples/clip.bgcode', clip],
  ['usb/examples/spacer.bgcode', spacer],
  ['usb/gearbox-housing.bgcode', gearbox],
  ['usb/phone-stand.bgcode', phoneStand],
  ['usb/spool-holder.bgcode', spoolHolder],
]);

export const devicePreviewOf = (
  _id: string,
  path: string,
): string | undefined => previews.get(path.replace(/^\//, ''));
