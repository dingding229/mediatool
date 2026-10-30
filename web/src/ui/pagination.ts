export const pageSizes = [10, 25, 50, 100] as const;
export function standardPageSize(value: number) {
  return pageSizes.reduce<number>((nearest, size) => Math.abs(size-value)<Math.abs(nearest-value)?size:nearest,25);
}
