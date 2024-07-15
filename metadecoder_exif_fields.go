package imagemeta

var maxEXIFField uint16

// Source: https://exiftool.org/TagNames/EXIF.html
var exifFields = map[uint16]string{
	0x0001: "InteropIndex",
	0x0002: "InteropVersion",
	0x000b: "ProcessingSoftware",
	0x00fe: "SubfileType",
	0x00ff: "OldSubfileType",
	0x0100: "ImageWidth",
	0x0101: "ImageHeight",
	0x0102: "BitsPerSample",
	0x0103: "Compression",
	0x0106: "PhotometricInterpretation",
	0x0107: "Thresholding",
	0x0108: "CellWidth",
	0x0109: "CellLength",
	0x010a: "FillOrder",
	0x010d: "DocumentName",
	0x010e: "ImageDescription",
	0x010f: "Make",
	0x0110: "Model",
	0x0111: "StripOffsets OtherImageStart PreviewJXLStart StripOffsets PreviewImageStart PreviewImageStart JpgFromRawStart",
	0x0112: "Orientation",
	0x0115: "SamplesPerPixel",
	0x0116: "RowsPerStrip",
	0x0117: "StripByteCounts OtherImageLength PreviewJXLLength StripByteCounts PreviewImageLength PreviewImageLength JpgFromRawLength",
	0x0118: "MinSampleValue",
	0x0119: "MaxSampleValue",
	0x011a: "XResolution",
	0x011b: "YResolution",
	0x011c: "PlanarConfiguration",
	0x011d: "PageName",
	0x011e: "XPosition",
	0x011f: "YPosition",
	0x0120: "FreeOffsets",
	0x0121: "FreeByteCounts",
	0x0122: "GrayResponseUnit",
	0x0123: "GrayResponseCurve",
	0x0124: "T4Options",
	0x0125: "T6Options",
	0x0128: "ResolutionUnit",
	0x0129: "PageNumber",
	0x012c: "ColorResponseUnit",
	0x012d: "TransferFunction",
	0x0131: "Software",
	0x0132: "ModifyDate",
	0x013b: "Artist",
	0x013c: "HostComputer",
	0x013d: "Predictor",
	0x013e: "WhitePoint",
	0x013f: "PrimaryChromaticities",
	0x0140: "ColorMap",
	0x0141: "HalftoneHints",
	0x0142: "TileWidth",
	0x0143: "TileLength",
	0x0144: "TileOffsets",
	0x0145: "TileByteCounts",
	0x0146: "BadFaxLines",
	0x0147: "CleanFaxData",
	0x0148: "ConsecutiveBadFaxLines",
	0x014a: "SubIFD A100DataOffset",
	0x014c: "InkSet",
	0x014d: "InkNames",
	0x014e: "NumberofInks",
	0x0150: "DotRange",
	0x0151: "TargetPrinter",
	0x0152: "ExtraSamples",
	0x0153: "SampleFormat",
	0x0154: "SMinSampleValue",
	0x0155: "SMaxSampleValue",
	0x0156: "TransferRange",
	0x0157: "ClipPath",
	0x0158: "XClipPathUnits",
	0x0159: "YClipPathUnits",
	0x015a: "Indexed",
	0x015b: "JPEGTables",
	0x015f: "OPIProxy",
	0x0190: "GlobalParametersIFD",
	0x0191: "ProfileType",
	0x0192: "FaxProfile",
	0x0193: "CodingMethods",
	0x0194: "VersionYear",
	0x0195: "ModeNumber",
	0x01b1: "Decode",
	0x01b2: "DefaultImageColor",
	0x01b3: "T82Options",
	0x01b5: "JPEGTables",
	0x0200: "JPEGProc",
	0x0201: "ThumbnailOffset ThumbnailOffset ThumbnailOffset PreviewImageStart PreviewImageStart JpgFromRawStart JpgFromRawStart OtherImageStart OtherImageStart OtherImageStart",
	0x0202: "ThumbnailLength ThumbnailLength ThumbnailLength PreviewImageLength PreviewImageLength JpgFromRawLength JpgFromRawLength OtherImageLength OtherImageLength OtherImageLength",
	0x0203: "JPEGRestartInterval",
	0x0205: "JPEGLosslessPredictors",
	0x0206: "JPEGPointTransforms",
	0x0207: "JPEGQTables",
	0x0208: "JPEGDCTables",
	0x0209: "JPEGACTables",
	0x0211: "YCbCrCoefficients",
	0x0212: "YCbCrSubSampling",
	0x0213: "YCbCrPositioning",
	0x0214: "ReferenceBlackWhite",
	0x022f: "StripRowCounts",
	0x02bc: "ApplicationNotes",
	0x0303: "RenderingIntent",
	0x03e7: "USPTOMiscellaneous",
	0x1000: "RelatedImageFileFormat",
	0x1001: "RelatedImageWidth",
	0x1002: "RelatedImageHeight",
	0x4746: "Rating",
	0x4747: "XP_DIP_XML",
	0x4748: "StitchInfo",
	0x4749: "RatingPercent",
	0x5001: "ResolutionXUnit",
	0x5002: "ResolutionYUnit",
	0x5003: "ResolutionXLengthUnit",
	0x5004: "ResolutionYLengthUnit",
	0x5005: "PrintFlags",
	0x5006: "PrintFlagsVersion",
	0x5007: "PrintFlagsCrop",
	0x5008: "PrintFlagsBleedWidth",
	0x5009: "PrintFlagsBleedWidthScale",
	0x500a: "HalftoneLPI",
	0x500b: "HalftoneLPIUnit",
	0x500c: "HalftoneDegree",
	0x500d: "HalftoneShape",
	0x500e: "HalftoneMisc",
	0x500f: "HalftoneScreen",
	0x5010: "JPEGQuality",
	0x5011: "GridSize",
	0x5012: "ThumbnailFormat",
	0x5013: "ThumbnailWidth",
	0x5014: "ThumbnailHeight",
	0x5015: "ThumbnailColorDepth",
	0x5016: "ThumbnailPlanes",
	0x5017: "ThumbnailRawBytes",
	0x5018: "ThumbnailLength",
	0x5019: "ThumbnailCompressedSize",
	0x501a: "ColorTransferFunction",
	0x501b: "ThumbnailData",
	0x5020: "ThumbnailImageWidth",
	0x5021: "ThumbnailImageHeight",
	0x5022: "ThumbnailBitsPerSample",
	0x5023: "ThumbnailCompression",
	0x5024: "ThumbnailPhotometricInterp",
	0x5025: "ThumbnailDescription",
	0x5026: "ThumbnailEquipMake",
	0x5027: "ThumbnailEquipModel",
	0x5028: "ThumbnailStripOffsets",
	0x5029: "ThumbnailOrientation",
	0x502a: "ThumbnailSamplesPerPixel",
	0x502b: "ThumbnailRowsPerStrip",
	0x502c: "ThumbnailStripByteCounts",
	0x502d: "ThumbnailResolutionX",
	0x502e: "ThumbnailResolutionY",
	0x502f: "ThumbnailPlanarConfig",
	0x5030: "ThumbnailResolutionUnit",
	0x5031: "ThumbnailTransferFunction",
	0x5032: "ThumbnailSoftware",
	0x5033: "ThumbnailDateTime",
	0x5034: "ThumbnailArtist",
	0x5035: "ThumbnailWhitePoint",
	0x5036: "ThumbnailPrimaryChromaticities",
	0x5037: "ThumbnailYCbCrCoefficients",
	0x5038: "ThumbnailYCbCrSubsampling",
	0x5039: "ThumbnailYCbCrPositioning",
	0x503a: "ThumbnailRefBlackWhite",
	0x503b: "ThumbnailCopyright",
	0x5090: "LuminanceTable",
	0x5091: "ChrominanceTable",
	0x5100: "FrameDelay",
	0x5101: "LoopCount",
	0x5102: "GlobalPalette",
	0x5103: "IndexBackground",
	0x5104: "IndexTransparent",
	0x5110: "PixelUnits",
	0x5111: "PixelsPerUnitX",
	0x5112: "PixelsPerUnitY",
	0x5113: "PaletteHistogram",
	0x7000: "SonyRawFileType",
	0x7010: "SonyToneCurve",
	0x7031: "VignettingCorrection",
	0x7032: "VignettingCorrParams",
	0x7034: "ChromaticAberrationCorrection",
	0x7035: "ChromaticAberrationCorrParams",
	0x7036: "DistortionCorrection",
	0x7037: "DistortionCorrParams",
	0x7038: "SonyRawImageSize",
	0x7310: "BlackLevel",
	0x7313: "WB_RGGBLevels",
	0x74c7: "SonyCropTopLeft",
	0x74c8: "SonyCropSize",
	0x800d: "ImageID",
	0x80a3: "WangTag1",
	0x80a4: "WangAnnotation",
	0x80a5: "WangTag3",
	0x80a6: "WangTag4",
	0x80b9: "ImageReferencePoints",
	0x80ba: "RegionXformTackPoint",
	0x80bb: "WarpQuadrilateral",
	0x80bc: "AffineTransformMat",
	0x80e3: "Matteing",
	0x80e4: "DataType",
	0x80e5: "ImageDepth",
	0x80e6: "TileDepth",
	0x8214: "ImageFullWidth",
	0x8215: "ImageFullHeight",
	0x8216: "TextureFormat",
	0x8217: "WrapModes",
	0x8218: "FovCot",
	0x8219: "MatrixWorldToScreen",
	0x821a: "MatrixWorldToCamera",
	0x827d: "Model2",
	0x828d: "CFARepeatPatternDim",
	0x828e: "CFAPattern2",
	0x828f: "BatteryLevel",
	0x8290: "KodakIFD",
	0x8298: "Copyright",
	0x829a: "ExposureTime",
	0x829d: "FNumber",
	0x82a5: "MDFileTag",
	0x82a6: "MDScalePixel",
	0x82a7: "MDColorTable",
	0x82a8: "MDLabName",
	0x82a9: "MDSampleInfo",
	0x82aa: "MDPrepDate",
	0x82ab: "MDPrepTime",
	0x82ac: "MDFileUnits",
	0x830e: "PixelScale",
	0x8335: "AdventScale",
	0x8336: "AdventRevision",
	0x835c: "UIC1Tag",
	0x835d: "UIC2Tag",
	0x835e: "UIC3Tag",
	0x835f: "UIC4Tag",
	0x83bb: "IPTC-NAA",
	0x847e: "IntergraphPacketData",
	0x847f: "IntergraphFlagRegisters",
	0x8480: "IntergraphMatrix",
	0x8481: "INGRReserved",
	0x8482: "ModelTiePoint",
	0x84e0: "Site",
	0x84e1: "ColorSequence",
	0x84e2: "IT8Header",
	0x84e3: "RasterPadding",
	0x84e4: "BitsPerRunLength",
	0x84e5: "BitsPerExtendedRunLength",
	0x84e6: "ColorTable",
	0x84e7: "ImageColorIndicator",
	0x84e8: "BackgroundColorIndicator",
	0x84e9: "ImageColorValue",
	0x84ea: "BackgroundColorValue",
	0x84eb: "PixelIntensityRange",
	0x84ec: "TransparencyIndicator",
	0x84ed: "ColorCharacterization",
	0x84ee: "HCUsage",
	0x84ef: "TrapIndicator",
	0x84f0: "CMYKEquivalent",
	0x8546: "SEMInfo",
	0x8568: "AFCP_IPTC",
	0x85b8: "PixelMagicJBIGOptions",
	0x85d7: "JPLCartoIFD",
	0x85d8: "ModelTransform",
	0x8602: "WB_GRGBLevels",
	0x8606: "LeafData",
	0x8649: "PhotoshopSettings",
	0x8769: "ExifOffset",
	0x8773: "ICC_Profile",
	0x877f: "TIFF_FXExtensions",
	0x8780: "MultiProfiles",
	0x8781: "SharedData",
	0x8782: "T88Options",
	0x87ac: "ImageLayer",
	0x87af: "GeoTiffDirectory",
	0x87b0: "GeoTiffDoubleParams",
	0x87b1: "GeoTiffAsciiParams",
	0x87be: "JBIGOptions",
	0x8822: "ExposureProgram",
	0x8824: "SpectralSensitivity",
	0x8825: "GPSInfo",
	0x8827: "ISO",
	0x8828: "Opto-ElectricConvFactor",
	0x8829: "Interlace",
	0x882a: "TimeZoneOffset",
	0x882b: "SelfTimerMode",
	0x8830: "SensitivityType",
	0x8831: "StandardOutputSensitivity",
	0x8832: "RecommendedExposureIndex",
	0x8833: "ISOSpeed",
	0x8834: "ISOSpeedLatitudeyyy",
	0x8835: "ISOSpeedLatitudezzz",
	0x885c: "FaxRecvParams",
	0x885d: "FaxSubAddress",
	0x885e: "FaxRecvTime",
	0x8871: "FedexEDR",
	0x888a: "LeafSubIFD",
	0x9000: "ExifVersion",
	0x9003: "DateTimeOriginal",
	0x9004: "CreateDate",
	0x9009: "GooglePlusUploadCode",
	0x9010: "OffsetTime",
	0x9011: "OffsetTimeOriginal",
	0x9012: "OffsetTimeDigitized",
	0x9101: "ComponentsConfiguration",
	0x9102: "CompressedBitsPerPixel",
	0x9201: "ShutterSpeedValue",
	0x9202: "ApertureValue",
	0x9203: "BrightnessValue",
	0x9204: "ExposureCompensation",
	0x9205: "MaxApertureValue",
	0x9206: "SubjectDistance",
	0x9207: "MeteringMode",
	0x9208: "LightSource",
	0x9209: "Flash",
	0x920a: "FocalLength",
	0x920b: "FlashEnergy",
	0x920c: "SpatialFrequencyResponse",
	0x920d: "Noise",
	0x920e: "FocalPlaneXResolution",
	0x920f: "FocalPlaneYResolution",
	0x9210: "FocalPlaneResolutionUnit",
	0x9211: "ImageNumber",
	0x9212: "SecurityClassification",
	0x9213: "ImageHistory",
	0x9214: "SubjectArea",
	0x9215: "ExposureIndex",
	0x9216: "TIFF-EPStandardID",
	0x9217: "SensingMethod",
	0x923a: "CIP3DataFile",
	0x923b: "CIP3Sheet",
	0x923c: "CIP3Side",
	0x923f: "StoNits",
	0x927c: "MakerNoteApple MakerNoteNikon MakerNoteCanon MakerNoteCasio MakerNoteCasio2 MakerNoteDJIInfo MakerNoteDJI MakerNoteFLIR MakerNoteFujiFilm MakerNoteGE MakerNoteGE2 MakerNoteHasselblad MakerNoteHP MakerNoteHP2 MakerNoteHP4 MakerNoteHP6 MakerNoteISL MakerNoteJVC MakerNoteJVCText MakerNoteKodak1a MakerNoteKodak1b MakerNoteKodak2 MakerNoteKodak3 MakerNoteKodak4 MakerNoteKodak5 MakerNoteKodak6a MakerNoteKodak6b MakerNoteKodak7 MakerNoteKodak8a MakerNoteKodak8b MakerNoteKodak8c MakerNoteKodak9 MakerNoteKodak10 MakerNoteKodak11 MakerNoteKodak12 MakerNoteKodakUnknown MakerNoteKyocera MakerNoteMinolta MakerNoteMinolta2 MakerNoteMinolta3 MakerNoteMotorola MakerNoteNikon2 MakerNoteNikon3 MakerNoteNintendo MakerNoteOlympus MakerNoteOlympus2 MakerNoteOlympus3 MakerNoteLeica MakerNoteLeica2 MakerNoteLeica3 MakerNoteLeica4 MakerNoteLeica5 MakerNoteLeica6 MakerNoteLeica7 MakerNoteLeica8 MakerNoteLeica9 MakerNoteLeica10 MakerNotePanasonic MakerNotePanasonic2 MakerNotePanasonic3 MakerNotePentax MakerNotePentax2 MakerNotePentax3 MakerNotePentax4 MakerNotePentax5 MakerNotePentax6 MakerNotePhaseOne MakerNoteReconyx MakerNoteReconyx2 MakerNoteReconyx3 MakerNoteRicohPentax MakerNoteRicoh MakerNoteRicoh2 MakerNoteRicohText MakerNoteSamsung1a MakerNoteSamsung1b MakerNoteSamsung2 MakerNoteSanyo MakerNoteSanyoC4 MakerNoteSanyoPatch MakerNoteSigma MakerNoteSony MakerNoteSony2 MakerNoteSony3 MakerNoteSony4 MakerNoteSony5 MakerNoteSonyEricsson MakerNoteSonySRF MakerNoteUnknownText MakerNoteUnknownBinary MakerNoteUnknown",
	0x9286: "UserComment",
	0x9290: "SubSecTime",
	0x9291: "SubSecTimeOriginal",
	0x9292: "SubSecTimeDigitized",
	0x932f: "MSDocumentText",
	0x9330: "MSPropertySetStorage",
	0x9331: "MSDocumentTextPosition",
	0x935c: "ImageSourceData",
	0x9400: "AmbientTemperature",
	0x9401: "Humidity",
	0x9402: "Pressure",
	0x9403: "WaterDepth",
	0x9404: "Acceleration",
	0x9405: "CameraElevationAngle",
	0x9999: "XiaomiSettings",
	0x9a00: "XiaomiModel",
	0x9c9b: "XPTitle",
	0x9c9c: "XPComment",
	0x9c9d: "XPAuthor",
	0x9c9e: "XPKeywords",
	0x9c9f: "XPSubject",
	0xa000: "FlashpixVersion",
	0xa001: "ColorSpace",
	0xa002: "ExifImageWidth",
	0xa003: "ExifImageHeight",
	0xa004: "RelatedSoundFile",
	0xa005: "InteropOffset",
	0xa010: "SamsungRawPointersOffset",
	0xa011: "SamsungRawPointersLength",
	0xa101: "SamsungRawByteOrder",
	0xa102: "SamsungRawUnknown?",
	0xa20b: "FlashEnergy",
	0xa20c: "SpatialFrequencyResponse",
	0xa20d: "Noise",
	0xa20e: "FocalPlaneXResolution",
	0xa20f: "FocalPlaneYResolution",
	0xa210: "FocalPlaneResolutionUnit",
	0xa211: "ImageNumber",
	0xa212: "SecurityClassification",
	0xa213: "ImageHistory",
	0xa214: "SubjectLocation",
	0xa215: "ExposureIndex",
	0xa216: "TIFF-EPStandardID",
	0xa217: "SensingMethod",
	0xa300: "FileSource",
	0xa301: "SceneType",
	0xa302: "CFAPattern",
	0xa401: "CustomRendered",
	0xa402: "ExposureMode",
	0xa403: "WhiteBalance",
	0xa404: "DigitalZoomRatio",
	0xa405: "FocalLengthIn35mmFormat",
	0xa406: "SceneCaptureType",
	0xa407: "GainControl",
	0xa408: "Contrast",
	0xa409: "Saturation",
	0xa40a: "Sharpness",
	0xa40b: "DeviceSettingDescription",
	0xa40c: "SubjectDistanceRange",
	0xa420: "ImageUniqueID",
	0xa430: "OwnerName",
	0xa431: "SerialNumber",
	0xa432: "LensInfo",
	0xa433: "LensMake",
	0xa434: "LensModel",
	0xa435: "LensSerialNumber",
	0xa436: "Title",
	0xa437: "Photographer",
	0xa438: "ImageEditor",
	0xa439: "CameraFirmware",
	0xa43a: "RAWDevelopingSoftware",
	0xa43b: "ImageEditingSoftware",
	0xa43c: "MetadataEditingSoftware",
	0xa460: "CompositeImage",
	0xa461: "CompositeImageCount",
	0xa462: "CompositeImageExposureTimes",
	0xa480: "GDALMetadata",
	0xa481: "GDALNoData",
	0xa500: "Gamma",
	0xafc0: "ExpandSoftware",
	0xafc1: "ExpandLens",
	0xafc2: "ExpandFilm",
	0xafc3: "ExpandFilterLens",
	0xafc4: "ExpandScanner",
	0xafc5: "ExpandFlashLamp",
	0xb4c3: "HasselbladRawImage",
	0xbc01: "PixelFormat",
	0xbc02: "Transformation",
	0xbc03: "Uncompressed",
	0xbc04: "ImageType",
	0xbc80: "ImageWidth",
	0xbc81: "ImageHeight",
	0xbc82: "WidthResolution",
	0xbc83: "HeightResolution",
	0xbcc0: "ImageOffset",
	0xbcc1: "ImageByteCount",
	0xbcc2: "AlphaOffset",
	0xbcc3: "AlphaByteCount",
	0xbcc4: "ImageDataDiscard",
	0xbcc5: "AlphaDataDiscard",
	0xc427: "OceScanjobDesc",
	0xc428: "OceApplicationSelector",
	0xc429: "OceIDNumber",
	0xc42a: "OceImageLogic",
	0xc44f: "Annotations",
	0xc4a5: "PrintIM",
	0xc519: "HasselbladXML",
	0xc51b: "HasselbladExif",
	0xc573: "OriginalFileName",
	0xc580: "USPTOOriginalContentType",
	0xc5e0: "CR2CFAPattern",
	0xc612: "DNGVersion",
	0xc613: "DNGBackwardVersion",
	0xc614: "UniqueCameraModel",
	0xc615: "LocalizedCameraModel",
	0xc616: "CFAPlaneColor",
	0xc617: "CFALayout",
	0xc618: "LinearizationTable",
	0xc619: "BlackLevelRepeatDim",
	0xc61a: "BlackLevel",
	0xc61b: "BlackLevelDeltaH",
	0xc61c: "BlackLevelDeltaV",
	0xc61d: "WhiteLevel",
	0xc61e: "DefaultScale",
	0xc61f: "DefaultCropOrigin",
	0xc620: "DefaultCropSize",
	0xc621: "ColorMatrix1",
	0xc622: "ColorMatrix2",
	0xc623: "CameraCalibration1",
	0xc624: "CameraCalibration2",
	0xc625: "ReductionMatrix1",
	0xc626: "ReductionMatrix2",
	0xc627: "AnalogBalance",
	0xc628: "AsShotNeutral",
	0xc629: "AsShotWhiteXY",
	0xc62a: "BaselineExposure",
	0xc62b: "BaselineNoise",
	0xc62c: "BaselineSharpness",
	0xc62d: "BayerGreenSplit",
	0xc62e: "LinearResponseLimit",
	0xc62f: "CameraSerialNumber",
	0xc630: "DNGLensInfo",
	0xc631: "ChromaBlurRadius",
	0xc632: "AntiAliasStrength",
	0xc633: "ShadowScale",
	0xc634: "SR2Private DNGAdobeData MakerNotePentax MakerNotePentax5 MakerNoteRicohPentax MakerNoteDJIInfo DNGPrivateData",
	0xc635: "MakerNoteSafety",
	0xc640: "RawImageSegmentation",
	0xc65a: "CalibrationIlluminant1",
	0xc65b: "CalibrationIlluminant2",
	0xc65c: "BestQualityScale",
	0xc65d: "RawDataUniqueID",
	0xc660: "AliasLayerMetadata",
	0xc68b: "OriginalRawFileName",
	0xc68c: "OriginalRawFileData",
	0xc68d: "ActiveArea",
	0xc68e: "MaskedAreas",
	0xc68f: "AsShotICCProfile",
	0xc690: "AsShotPreProfileMatrix",
	0xc691: "CurrentICCProfile",
	0xc692: "CurrentPreProfileMatrix",
	0xc6bf: "ColorimetricReference",
	0xc6c5: "SRawType",
	0xc6d2: "PanasonicTitle",
	0xc6d3: "PanasonicTitle2",
	0xc6f3: "CameraCalibrationSig",
	0xc6f4: "ProfileCalibrationSig",
	0xc6f5: "ProfileIFD",
	0xc6f6: "AsShotProfileName",
	0xc6f7: "NoiseReductionApplied",
	0xc6f8: "ProfileName",
	0xc6f9: "ProfileHueSatMapDims",
	0xc6fa: "ProfileHueSatMapData1",
	0xc6fb: "ProfileHueSatMapData2",
	0xc6fc: "ProfileToneCurve",
	0xc6fd: "ProfileEmbedPolicy",
	0xc6fe: "ProfileCopyright",
	0xc714: "ForwardMatrix1",
	0xc715: "ForwardMatrix2",
	0xc716: "PreviewApplicationName",
	0xc717: "PreviewApplicationVersion",
	0xc718: "PreviewSettingsName",
	0xc719: "PreviewSettingsDigest",
	0xc71a: "PreviewColorSpace",
	0xc71b: "PreviewDateTime",
	0xc71c: "RawImageDigest",
	0xc71d: "OriginalRawFileDigest",
	0xc71e: "SubTileBlockSize",
	0xc71f: "RowInterleaveFactor",
	0xc725: "ProfileLookTableDims",
	0xc726: "ProfileLookTableData",
	0xc740: "OpcodeList1",
	0xc741: "OpcodeList2",
	0xc74e: "OpcodeList3",
	0xc761: "NoiseProfile",
	0xc763: "TimeCodes",
	0xc764: "FrameRate",
	0xc772: "TStop",
	0xc789: "ReelName",
	0xc791: "OriginalDefaultFinalSize",
	0xc792: "OriginalBestQualitySize",
	0xc793: "OriginalDefaultCropSize",
	0xc7a1: "CameraLabel",
	0xc7a3: "ProfileHueSatMapEncoding",
	0xc7a4: "ProfileLookTableEncoding",
	0xc7a5: "BaselineExposureOffset",
	0xc7a6: "DefaultBlackRender",
	0xc7a7: "NewRawImageDigest",
	0xc7a8: "RawToPreviewGain",
	0xc7aa: "CacheVersion",
	0xc7b5: "DefaultUserCrop",
	0xc7d5: "NikonNEFInfo",
	0xc7e9: "DepthFormat",
	0xc7ea: "DepthNear",
	0xc7eb: "DepthFar",
	0xc7ec: "DepthUnits",
	0xc7ed: "DepthMeasureType",
	0xc7ee: "EnhanceParams",
	0xcd2d: "ProfileGainTableMap",
	0xcd2e: "SemanticName",
	0xcd30: "SemanticInstanceID",
	0xcd31: "CalibrationIlluminant3",
	0xcd32: "CameraCalibration3",
	0xcd33: "ColorMatrix3",
	0xcd34: "ForwardMatrix3",
	0xcd35: "IlluminantData1",
	0xcd36: "IlluminantData2",
	0xcd37: "IlluminantData3",
	0xcd38: "MaskSubArea",
	0xcd39: "ProfileHueSatMapData3",
	0xcd3a: "ReductionMatrix3",
	0xcd3f: "RGBTables",
	0xcd40: "ProfileGainTableMap2",
	0xcd41: "JUMBF",
	0xcd43: "ColumnInterleaveFactor",
	0xcd44: "ImageSequenceInfo",
	0xcd46: "ImageStats",
	0xcd47: "ProfileDynamicRange",
	0xcd48: "ProfileGroupName",
	0xcd49: "JXLDistance",
	0xcd4a: "JXLEffort",
	0xcd4b: "JXLDecodeSpeed",
	0xea1c: "Padding",
	0xea1d: "OffsetSchema",
	0xfde8: "OwnerName",
	0xfde9: "SerialNumber",
	0xfdea: "Lens",
	0xfe00: "KDC_IFD",
	0xfe4c: "RawFile",
	0xfe4d: "Converter",
	0xfe4e: "WhiteBalance",
	0xfe51: "Exposure",
	0xfe52: "Shadows",
	0xfe53: "Brightness",
	0xfe54: "Contrast",
	0xfe55: "Saturation",
	0xfe56: "Sharpness",
	0xfe57: "Smoothness",
	0xfe58: "MoireFilter",
}

var exifFieldsGPS = map[uint16]string{
	0x0000: "GPSVersionID",
	0x0001: "GPSLatitudeRef",
	0x0002: "GPSLatitude",
	0x0003: "GPSLongitudeRef",
	0x0004: "GPSLongitude",
	0x0005: "GPSAltitudeRef",
	0x0006: "GPSAltitude",
	0x0007: "GPSTimeStamp",
	0x0008: "GPSSatellites",
	0x0009: "GPSStatus",
	0x000a: "GPSMeasureMode",
	0x000b: "GPSDOP",
	0x000c: "GPSSpeedRef",
	0x000d: "GPSSpeed",
	0x000e: "GPSTrackRef",
	0x000f: "GPSTrack",
	0x0010: "GPSImgDirectionRef",
	0x0011: "GPSImgDirection",
	0x0012: "GPSMapDatum",
	0x0013: "GPSDestLatitudeRef",
	0x0014: "GPSDestLatitude",
	0x0015: "GPSDestLongitudeRef",
	0x0016: "GPSDestLongitude",
	0x0017: "GPSDestBearingRef",
	0x0018: "GPSDestBearing",
	0x0019: "GPSDestDistanceRef",
	0x001a: "GPSDestDistance",
	0x001b: "GPSProcessingMethod",
	0x001c: "GPSAreaInformation",
	0x001d: "GPSDateStamp",
	0x001e: "GPSDifferential",
	0x001f: "GPSHPositioningError",
}
