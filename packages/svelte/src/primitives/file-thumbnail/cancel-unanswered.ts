/**
 * An attachment for an `<img>`: when the element goes before its picture
 * answered, its request goes with it. Dropping the address is what makes a
 * browser cancel an image request; taking the element out of the document
 * does not, and a request left running holds one of the page's few
 * connections.
 */
export const cancelUnanswered = (image: HTMLImageElement): (() => void) => {
  let answered = false;
  const settle = (): void => {
    answered = true;
  };
  image.addEventListener('load', settle);
  image.addEventListener('error', settle);

  return () => {
    image.removeEventListener('load', settle);
    image.removeEventListener('error', settle);
    if (!answered) image.removeAttribute('src');
  };
};
