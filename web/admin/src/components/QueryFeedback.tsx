import { Banner, Button, Stack, Text } from '@astryxdesign/core'

/** Report transport state separately from the values in the last good snapshot. */
export default function QueryFeedback({
  query,
  label,
}: {
  query: {
    isFetching: boolean
    isError: boolean
    dataUpdatedAt: number
    error: Error | null
    refetch: () => unknown
  }
  label: string
}) {
  return (
    <Stack gap={2}>
      {query.isError && (
        <Banner
          status="warning"
          title={`${label} could not refresh`}
          description={
            query.dataUpdatedAt
              ? 'Showing the last successful snapshot. Values may be out of date.'
              : 'Data is unavailable. Retry to load it.'
          }
        />
      )}
      <Stack direction="horizontal" gap={2} align="center" wrap="wrap">
        <Text type="supporting" role="status">
          {query.isFetching
            ? 'Refreshing…'
            : query.dataUpdatedAt
              ? `Last updated ${new Date(query.dataUpdatedAt).toLocaleString()}`
              : 'No successful update yet'}
        </Text>
        <Button
          size="sm"
          variant="secondary"
          label={query.isError ? 'Retry' : 'Refresh'}
          isDisabled={query.isFetching}
          onClick={() => {
            void query.refetch()
          }}
        />
      </Stack>
    </Stack>
  )
}
