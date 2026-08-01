import 'dotenv/config'
import { LangfuseSpanProcessor } from '@langfuse/otel'
import { NodeTracerProvider } from '@opentelemetry/sdk-trace-node'

/**
 * Must load before application code so all spans export to Langfuse.
 * Credentials: LANGFUSE_PUBLIC_KEY, LANGFUSE_SECRET_KEY, LANGFUSE_BASE_URL
 */
export const langfuseSpanProcessor = new LangfuseSpanProcessor()

const provider = new NodeTracerProvider({
  spanProcessors: [langfuseSpanProcessor],
})

provider.register()

export async function flushLangfuse() {
  await langfuseSpanProcessor.forceFlush()
}
