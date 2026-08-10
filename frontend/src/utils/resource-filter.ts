export function filterResourcesByName<T extends { metadata: { name: string } }>(items: T[], query: string): T[] {
  const normalizedQuery = query.trim().toLowerCase()
  if (!normalizedQuery) return items
  return items.filter((item) => item.metadata.name.toLowerCase().includes(normalizedQuery))
}
