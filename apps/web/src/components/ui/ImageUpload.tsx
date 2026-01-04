import React, { useState, useRef, useCallback } from 'react';
import { uploadImage, type UploadStatusResponse } from '../../lib/mediaApi';
import './ImageUpload.css';

interface ImageUploadProps {
    purpose: 'avatar' | 'event_cover';
    onUploadComplete: (result: UploadStatusResponse) => void;
    onError?: (error: Error) => void;
    maxSizeMB?: number;
    accept?: string;
    className?: string;
    children?: React.ReactNode;
    preview?: string; // Current image URL
}

type UploadPhase = 'idle' | 'uploading' | 'processing' | 'complete' | 'error';

export const ImageUpload: React.FC<ImageUploadProps> = ({
    purpose,
    onUploadComplete,
    onError,
    maxSizeMB = 10,
    accept = 'image/jpeg,image/png,image/webp',
    className = '',
    children,
    preview,
}) => {
    const [phase, setPhase] = useState<UploadPhase>('idle');
    const [progress, setProgress] = useState(0);
    const [previewUrl, setPreviewUrl] = useState<string | null>(preview || null);
    const [error, setError] = useState<string | null>(null);
    const inputRef = useRef<HTMLInputElement>(null);

    const handleFileSelect = useCallback(async (file: File) => {
        // Validate file size
        if (file.size > maxSizeMB * 1024 * 1024) {
            const errorMsg = `File too large. Max size is ${maxSizeMB}MB.`;
            setError(errorMsg);
            setPhase('error');
            onError?.(new Error(errorMsg));
            return;
        }

        // Validate file type
        if (!accept.split(',').some(type => file.type === type.trim())) {
            const errorMsg = 'Invalid file type. Please upload JPEG, PNG, or WebP.';
            setError(errorMsg);
            setPhase('error');
            onError?.(new Error(errorMsg));
            return;
        }

        // Create preview
        const objectUrl = URL.createObjectURL(file);
        setPreviewUrl(objectUrl);
        setError(null);
        setPhase('uploading');
        setProgress(0);

        try {
            const result = await uploadImage(file, purpose, (prog, uploadPhase) => {
                setProgress(prog);
                setPhase(uploadPhase);
            });

            if (result.status === 'READY') {
                setPhase('complete');
                // Update preview to derived URL
                if (result.derived_urls) {
                    const bestSize = purpose === 'avatar' ? '512' : '800';
                    setPreviewUrl(result.derived_urls[bestSize] || Object.values(result.derived_urls)[0]);
                }
                onUploadComplete(result);
            } else {
                throw new Error(result.error || 'Upload failed');
            }
        } catch (err) {
            const errorMsg = err instanceof Error ? err.message : 'Upload failed';
            setError(errorMsg);
            setPhase('error');
            onError?.(err instanceof Error ? err : new Error(errorMsg));
        } finally {
            URL.revokeObjectURL(objectUrl);
        }
    }, [purpose, maxSizeMB, accept, onUploadComplete, onError]);

    const handleInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
        const file = e.target.files?.[0];
        if (file) {
            handleFileSelect(file);
        }
        // Reset input
        if (inputRef.current) {
            inputRef.current.value = '';
        }
    };

    const handleDrop = (e: React.DragEvent) => {
        e.preventDefault();
        e.stopPropagation();

        const file = e.dataTransfer.files?.[0];
        if (file) {
            handleFileSelect(file);
        }
    };

    const handleDragOver = (e: React.DragEvent) => {
        e.preventDefault();
        e.stopPropagation();
    };

    const triggerFileSelect = () => {
        if (phase !== 'uploading' && phase !== 'processing') {
            inputRef.current?.click();
        }
    };

    return (
        <div
            className={`image-upload ${purpose} ${phase} ${className}`}
            onDrop={handleDrop}
            onDragOver={handleDragOver}
        >
            <input
                ref={inputRef}
                type="file"
                accept={accept}
                onChange={handleInputChange}
                hidden
            />

            <div className="image-upload-content" onClick={triggerFileSelect}>
                {previewUrl ? (
                    <div className="image-upload-preview">
                        <img src={previewUrl} alt="Preview" />
                        {phase === 'idle' && (
                            <div className="image-upload-overlay">
                                <span className="image-upload-edit-icon">✏️</span>
                                <span>Change</span>
                            </div>
                        )}
                    </div>
                ) : (
                    <div className="image-upload-placeholder">
                        {children || (
                            <>
                                <span className="image-upload-icon">📷</span>
                                <span>Click or drop to upload</span>
                            </>
                        )}
                    </div>
                )}

                {(phase === 'uploading' || phase === 'processing') && (
                    <div className="image-upload-progress">
                        <div className="image-upload-progress-bar">
                            <div
                                className="image-upload-progress-fill"
                                style={{ width: `${progress}%` }}
                            />
                        </div>
                        <span className="image-upload-progress-text">
                            {phase === 'uploading' ? `Uploading... ${progress}%` : 'Processing...'}
                        </span>
                    </div>
                )}

                {phase === 'error' && (
                    <div className="image-upload-error">
                        <span>❌ {error}</span>
                        <button onClick={(e) => { e.stopPropagation(); setPhase('idle'); setError(null); }}>
                            Try again
                        </button>
                    </div>
                )}
            </div>
        </div>
    );
};

export default ImageUpload;
