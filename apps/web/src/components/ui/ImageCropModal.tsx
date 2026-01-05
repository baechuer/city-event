import React, { useState, useRef, useCallback } from 'react';
import ReactCrop, { type Crop, type PixelCrop, centerCrop, makeAspectCrop } from 'react-image-crop';
import 'react-image-crop/dist/ReactCrop.css';
import './ImageCropModal.css';

interface ImageCropModalProps {
    imageSrc: string;
    aspectRatio?: number; // e.g., 1 for 1:1, 16/9 for 16:9
    onCropComplete: (croppedBlob: Blob) => void;
    onCancel: () => void;
    minWidth?: number;
    minHeight?: number;
}

function centerAspectCrop(
    mediaWidth: number,
    mediaHeight: number,
    aspect: number,
): Crop {
    return centerCrop(
        makeAspectCrop(
            {
                unit: '%',
                width: 90,
            },
            aspect,
            mediaWidth,
            mediaHeight,
        ),
        mediaWidth,
        mediaHeight,
    );
}

export const ImageCropModal: React.FC<ImageCropModalProps> = ({
    imageSrc,
    aspectRatio = 1,
    onCropComplete,
    onCancel,
    minWidth = 100,
    minHeight = 100,
}) => {
    const [crop, setCrop] = useState<Crop>();
    const [completedCrop, setCompletedCrop] = useState<PixelCrop>();
    const imgRef = useRef<HTMLImageElement>(null);
    const [isProcessing, setIsProcessing] = useState(false);

    const onImageLoad = useCallback((e: React.SyntheticEvent<HTMLImageElement>) => {
        const { width, height } = e.currentTarget;
        setCrop(centerAspectCrop(width, height, aspectRatio));
    }, [aspectRatio]);

    const getCroppedBlob = useCallback(async (): Promise<Blob | null> => {
        const image = imgRef.current;
        if (!image || !completedCrop) {
            return null;
        }

        const canvas = document.createElement('canvas');
        const scaleX = image.naturalWidth / image.width;
        const scaleY = image.naturalHeight / image.height;

        canvas.width = completedCrop.width * scaleX;
        canvas.height = completedCrop.height * scaleY;

        const ctx = canvas.getContext('2d');
        if (!ctx) {
            return null;
        }

        ctx.drawImage(
            image,
            completedCrop.x * scaleX,
            completedCrop.y * scaleY,
            completedCrop.width * scaleX,
            completedCrop.height * scaleY,
            0,
            0,
            canvas.width,
            canvas.height,
        );

        return new Promise((resolve) => {
            canvas.toBlob(
                (blob) => resolve(blob),
                'image/jpeg',
                0.9
            );
        });
    }, [completedCrop]);

    const handleConfirm = async () => {
        setIsProcessing(true);
        try {
            const blob = await getCroppedBlob();
            if (blob) {
                onCropComplete(blob);
            }
        } finally {
            setIsProcessing(false);
        }
    };

    return (
        <div className="image-crop-modal-overlay" onClick={onCancel}>
            <div className="image-crop-modal" onClick={(e) => e.stopPropagation()}>
                <div className="image-crop-modal-header">
                    <h3>Crop Image</h3>
                    <button className="image-crop-close" onClick={onCancel}>×</button>
                </div>

                <div className="image-crop-container">
                    <ReactCrop
                        crop={crop}
                        onChange={(_, percentCrop) => setCrop(percentCrop)}
                        onComplete={(c) => setCompletedCrop(c)}
                        aspect={aspectRatio}
                        minWidth={minWidth}
                        minHeight={minHeight}
                    >
                        <img
                            ref={imgRef}
                            src={imageSrc}
                            alt="Crop preview"
                            onLoad={onImageLoad}
                            style={{ maxHeight: '60vh', maxWidth: '100%' }}
                        />
                    </ReactCrop>
                </div>

                <div className="image-crop-modal-footer">
                    <button
                        className="image-crop-btn image-crop-btn-secondary"
                        onClick={onCancel}
                        disabled={isProcessing}
                    >
                        Cancel
                    </button>
                    <button
                        className="image-crop-btn image-crop-btn-primary"
                        onClick={handleConfirm}
                        disabled={isProcessing || !completedCrop}
                    >
                        {isProcessing ? 'Processing...' : 'Apply Crop'}
                    </button>
                </div>
            </div>
        </div>
    );
};

export default ImageCropModal;
