export const UNKNOWN_REGION_LABEL = 'Регион не указан'

export function getRegionLabel(
  regionId: string | null | undefined,
  regionNames: ReadonlyMap<string, string>,
) {
  if (!regionId) return UNKNOWN_REGION_LABEL
  return regionNames.get(regionId) || UNKNOWN_REGION_LABEL
}
