export interface CompressedImageResult {
  file: File;
  originalSizeBytes: number;
  compressedSizeBytes: number;
  wasCompressed: boolean;
}

/**
 * Compresses large camera photos in the browser before uploading to private presigned storage,
 * keeping text on identity documents (KK/Akta/Ijazah) legible under ~1 MB.
 */
export async function compressImageForUpload(
  file: File,
  options: { maxDimension?: number; quality?: number; skipBelowBytes?: number } = {},
): Promise<CompressedImageResult> {
  const maxDimension = options.maxDimension ?? 1600;
  const quality = options.quality ?? 0.82;
  const skipBelowBytes = options.skipBelowBytes ?? 750 * 1024;

  if (!file.type.startsWith('image/') || file.type === 'image/svg+xml' || file.size <= skipBelowBytes) {
    return {
      file,
      originalSizeBytes: file.size,
      compressedSizeBytes: file.size,
      wasCompressed: false,
    };
  }

  const bitmap = await createImageBitmap(file);
  try {
    const scale = Math.min(1, maxDimension / Math.max(bitmap.width, bitmap.height));
    const width = Math.max(1, Math.round(bitmap.width * scale));
    const height = Math.max(1, Math.round(bitmap.height * scale));

    const canvas = document.createElement('canvas');
    canvas.width = width;
    canvas.height = height;
    const ctx = canvas.getContext('2d');
    if (!ctx) {
      return {
        file,
        originalSizeBytes: file.size,
        compressedSizeBytes: file.size,
        wasCompressed: false,
      };
    }

    ctx.drawImage(bitmap, 0, 0, width, height);

    const blob = await new Promise<Blob | null>((resolve) =>
      canvas.toBlob(resolve, 'image/jpeg', quality),
    );

    if (!blob || blob.size >= file.size) {
      return {
        file,
        originalSizeBytes: file.size,
        compressedSizeBytes: file.size,
        wasCompressed: false,
      };
    }

    const compressedName = file.name.replace(/\.[^.]+$/, '') + '.jpg';
    const compressedFile = new File([blob], compressedName, {
      type: 'image/jpeg',
      lastModified: Date.now(),
    });

    return {
      file: compressedFile,
      originalSizeBytes: file.size,
      compressedSizeBytes: compressedFile.size,
      wasCompressed: true,
    };
  } finally {
    bitmap.close();
  }
}

