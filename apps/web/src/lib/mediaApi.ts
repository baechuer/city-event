import { apiClient } from './apiClient';

export interface RequestUploadResponse {
    upload_id: string;
    presigned_url: string;
    object_key: string;
    expires_at: string;
}

export interface UploadStatusResponse {
    id: string;
    status: 'PENDING' | 'UPLOADED' | 'PROCESSING' | 'READY' | 'FAILED';
    derived_urls?: Record<string, string>;
    error?: string;
}

/**
 * Request a presigned URL for uploading an image.
 * @param purpose - 'avatar' or 'event_cover'
 */
export async function requestUpload(purpose: 'avatar' | 'event_cover'): Promise<RequestUploadResponse> {
    const response = await apiClient.post<RequestUploadResponse>('/media/request-upload', { purpose });
    return response.data;
}

/**
 * Upload file to presigned URL.
 * @param presignedUrl - The presigned PUT URL
 * @param file - The file to upload
 * @param onProgress - Optional progress callback (0-100)
 */
export async function uploadToPresignedUrl(
    presignedUrl: string,
    file: File,
    onProgress?: (progress: number) => void
): Promise<void> {
    const xhr = new XMLHttpRequest();

    return new Promise((resolve, reject) => {
        xhr.upload.addEventListener('progress', (event) => {
            if (event.lengthComputable && onProgress) {
                const progress = Math.round((event.loaded / event.total) * 100);
                onProgress(progress);
            }
        });

        xhr.addEventListener('load', () => {
            if (xhr.status >= 200 && xhr.status < 300) {
                resolve();
            } else {
                reject(new Error(`Upload failed with status ${xhr.status}`));
            }
        });

        xhr.addEventListener('error', () => {
            reject(new Error('Upload failed'));
        });

        xhr.open('PUT', presignedUrl);
        xhr.setRequestHeader('Content-Type', file.type);
        xhr.send(file);
    });
}

/**
 * Mark upload as complete and trigger processing.
 * @param uploadId - The upload ID from requestUpload
 */
export async function completeUpload(uploadId: string): Promise<{ status: string }> {
    const response = await apiClient.post<{ status: string }>('/media/complete', { upload_id: uploadId });
    return response.data;
}

/**
 * Poll upload status until READY or FAILED.
 * @param uploadId - The upload ID
 * @param onStatusChange - Callback when status changes
 * @param maxAttempts - Max polling attempts (default 30)
 * @param intervalMs - Polling interval in ms (default 1000)
 */
export async function pollUploadStatus(
    uploadId: string,
    onStatusChange?: (status: UploadStatusResponse) => void,
    maxAttempts = 30,
    intervalMs = 1000
): Promise<UploadStatusResponse> {
    for (let i = 0; i < maxAttempts; i++) {
        const response = await apiClient.get<UploadStatusResponse>(`/media/${uploadId}/status`);
        const status = response.data;

        if (onStatusChange) {
            onStatusChange(status);
        }

        if (status.status === 'READY' || status.status === 'FAILED') {
            return status;
        }

        await new Promise(resolve => setTimeout(resolve, intervalMs));
    }

    throw new Error('Upload processing timed out');
}

/**
 * Complete upload flow: request URL, upload, complete, poll for result.
 */
export async function uploadImage(
    file: File,
    purpose: 'avatar' | 'event_cover',
    onProgress?: (progress: number, phase: 'uploading' | 'processing') => void
): Promise<UploadStatusResponse> {
    // 1. Request presigned URL
    const { upload_id, presigned_url } = await requestUpload(purpose);

    // 2. Upload to presigned URL
    await uploadToPresignedUrl(presigned_url, file, (progress) => {
        onProgress?.(progress, 'uploading');
    });

    // 3. Mark complete
    await completeUpload(upload_id);

    // 4. Poll for result
    onProgress?.(0, 'processing');
    const result = await pollUploadStatus(upload_id, (status) => {
        if (status.status === 'PROCESSING') {
            onProgress?.(50, 'processing');
        }
    });

    onProgress?.(100, 'processing');
    return result;
}
