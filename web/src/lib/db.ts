/**
 * 轻量 IndexedDB 封装（无第三方依赖）。
 *
 * 三个 object store 互相隔离（N-08 / N-27）：
 *  - favorites  收藏缓存
 *  - user_state 用户状态与配置缓存
 *  - app_logs   前端分类日志（独立 store，便于单独清理与容量控制）
 */

const DB_NAME = 'lanreel'
const DB_VERSION = 1

export type StoreName = 'favorites' | 'user_state' | 'app_logs'

let dbPromise: Promise<IDBDatabase> | null = null

function openDB(): Promise<IDBDatabase> {
  if (dbPromise) return dbPromise
  dbPromise = new Promise((resolve, reject) => {
    if (typeof indexedDB === 'undefined') {
      reject(new Error('当前浏览器不支持 IndexedDB'))
      return
    }
    const req = indexedDB.open(DB_NAME, DB_VERSION)
    req.onupgradeneeded = () => {
      const db = req.result
      if (!db.objectStoreNames.contains('favorites')) {
        db.createObjectStore('favorites', { keyPath: 'key' })
      }
      if (!db.objectStoreNames.contains('user_state')) {
        db.createObjectStore('user_state', { keyPath: 'key' })
      }
      if (!db.objectStoreNames.contains('app_logs')) {
        const store = db.createObjectStore('app_logs', { keyPath: 'id', autoIncrement: true })
        store.createIndex('ts', 'ts')
        store.createIndex('category', 'category')
        store.createIndex('level', 'level')
      }
    }
    req.onsuccess = () => resolve(req.result)
    req.onerror = () => reject(req.error ?? new Error('IndexedDB 打开失败'))
  })
  return dbPromise
}

async function withStore<T>(
  store: StoreName,
  mode: IDBTransactionMode,
  fn: (store: IDBObjectStore) => IDBRequest<T>,
): Promise<T> {
  const db = await openDB()
  return new Promise<T>((resolve, reject) => {
    const tx = db.transaction(store, mode)
    const req = fn(tx.objectStore(store))
    req.onsuccess = () => resolve(req.result)
    req.onerror = () => reject(req.error ?? new Error('IndexedDB 操作失败'))
  })
}

export async function putItem<T extends object>(store: StoreName, value: T): Promise<void> {
  await withStore(store, 'readwrite', (s) => s.put(value as unknown as never))
}

export async function putMany<T extends object>(store: StoreName, values: T[]): Promise<void> {
  if (!values.length) return
  const db = await openDB()
  await new Promise<void>((resolve, reject) => {
    const tx = db.transaction(store, 'readwrite')
    const os = tx.objectStore(store)
    values.forEach((v) => os.put(v as unknown as never))
    tx.oncomplete = () => resolve()
    tx.onerror = () => reject(tx.error ?? new Error('IndexedDB 批量写入失败'))
  })
}

export async function getItem<T>(store: StoreName, key: IDBValidKey): Promise<T | undefined> {
  return withStore<T | undefined>(store, 'readonly', (s) => s.get(key) as IDBRequest<T | undefined>)
}

export async function deleteItem(store: StoreName, key: IDBValidKey): Promise<void> {
  await withStore(store, 'readwrite', (s) => s.delete(key))
}

export async function clearStore(store: StoreName): Promise<void> {
  await withStore(store, 'readwrite', (s) => s.clear())
}

export async function getAll<T>(store: StoreName): Promise<T[]> {
  return withStore<T[]>(store, 'readonly', (s) => s.getAll() as IDBRequest<T[]>)
}

export async function countAll(store: StoreName): Promise<number> {
  return withStore<number>(store, 'readonly', (s) => s.count())
}

/** 查询 app_logs，按时间倒序。 */
export async function queryLogs(options: {
  category?: string
  level?: string
  keyword?: string
  limit?: number
}): Promise<import('./logger').LogRecord[]> {
  const all = await getAll<import('./logger').LogRecord>('app_logs')
  const kw = (options.keyword ?? '').toLowerCase()
  const filtered = all
    .filter((r) => (options.category ? r.category === options.category : true))
    .filter((r) => (options.level ? r.level === options.level : true))
    .filter((r) =>
      kw
        ? r.message.toLowerCase().includes(kw) ||
          r.module.toLowerCase().includes(kw) ||
          JSON.stringify(r.context ?? {}).toLowerCase().includes(kw)
        : true,
    )
    .sort((a, b) => (a.ts < b.ts ? 1 : -1))
  return options.limit ? filtered.slice(0, options.limit) : filtered
}

/** 按时间淘汰最旧日志，保持在容量上限内。 */
export async function trimLogs(max: number): Promise<void> {
  const total = await countAll('app_logs')
  if (total <= max) return
  const all = await getAll<import('./logger').LogRecord>('app_logs')
  const remove = all
    .sort((a, b) => (a.ts < b.ts ? -1 : 1))
    .slice(0, total - max)
    .map((r) => r.id!)
    .filter((id) => id != null)
  const db = await openDB()
  await new Promise<void>((resolve, reject) => {
    const tx = db.transaction('app_logs', 'readwrite')
    const os = tx.objectStore('app_logs')
    remove.forEach((id) => os.delete(id))
    tx.oncomplete = () => resolve()
    tx.onerror = () => reject(tx.error ?? new Error('IndexedDB 清理失败'))
  })
}
